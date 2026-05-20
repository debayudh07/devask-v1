import os
import json
from contextlib import asynccontextmanager
from fastapi import FastAPI, HTTPException
from pydantic import BaseModel
from typing import List, Dict, Any
from dotenv import load_dotenv

from pinecone import Pinecone, ServerlessSpec
from sentence_transformers import SentenceTransformer, CrossEncoder
from rank_bm25 import BM25Okapi

load_dotenv()

# ─── Configuration ────────────────────────────────────────────────────────────
PINECONE_API_KEY = os.getenv("PINECONE_API_KEY")
INDEX_NAME       = os.getenv("PINECONE_INDEX_NAME", "devask-knowledge")
EMBED_MODEL_NAME = "BAAI/bge-base-en-v1.5"
RERANK_MODEL_NAME = "cross-encoder/ms-marco-MiniLM-L-6-v2"
DATA_DIR = os.getenv("DEVASK_DATA_DIR", os.path.join(os.path.dirname(__file__), "data"))
BM25_STORE_PATH = os.path.join(DATA_DIR, "bm25_stores.json")

# ─── Globals ──────────────────────────────────────────────────────────────────
pc            = None   # Pinecone client
index         = None   # Pinecone index handle
embed_model   = None   # SentenceTransformer for dense embeddings
cross_encoder = None   # CrossEncoder for reranking

# BM25 store: team_id → { "bm25": BM25Okapi, "corpus": List[str], "ids": List[str], "filenames": set }
bm25_stores: Dict[str, Dict] = {}


# ─── Lifespan ─────────────────────────────────────────────────────────────────
@asynccontextmanager
async def lifespan(app: FastAPI):
    global pc, index, embed_model, cross_encoder

    print("=== Devask AI Microservice starting ===")

    _load_bm25_stores()

    # 1. Load embedding model
    print(f"  Loading embedding model  : {EMBED_MODEL_NAME}")
    embed_model = SentenceTransformer(EMBED_MODEL_NAME)

    # 2. Load reranking model (initialise once; reuse across requests)
    print(f"  Loading reranking model  : {RERANK_MODEL_NAME}")
    cross_encoder = CrossEncoder(RERANK_MODEL_NAME)

    # 3. Connect to Pinecone
    if not PINECONE_API_KEY:
        print("  WARNING: PINECONE_API_KEY not set — vector search disabled!")
    else:
        print("  Connecting to Pinecone...")
        pc = Pinecone(api_key=PINECONE_API_KEY)

        existing = [i["name"] for i in pc.list_indexes()]
        if INDEX_NAME not in existing:
            print(f"  Creating Pinecone index  : {INDEX_NAME}")
            pc.create_index(
                name=INDEX_NAME,
                dimension=768,
                metric="cosine",
                spec=ServerlessSpec(cloud="aws", region="us-east-1"),
            )
        index = pc.Index(INDEX_NAME)
        print("  Pinecone ready.")

    print("=== Microservice ready ===\n")
    yield
    print("Shutting down AI Microservice.")


app = FastAPI(
    title="Devask AI Microservice",
    lifespan=lifespan,
    docs_url=None,    # disable /docs (Swagger UI) to avoid route conflict
    redoc_url=None,   # disable /redoc too
)


# ─── Pydantic Models ──────────────────────────────────────────────────────────
class Document(BaseModel):
    id: str
    text: str
    metadata: Dict[str, Any]

class EmbedRequest(BaseModel):
    team_id: str
    documents: List[Document]

class QueryRequest(BaseModel):
    team_id: str
    query: str
    top_k: int = 10

class HybridQueryRequest(BaseModel):
    team_id: str
    query: str
    top_k: int = 10          # final number of results after fusion
    semantic_weight: float = 0.6   # weight for semantic scores in RRF (0–1)

class RerankRequest(BaseModel):
    query: str
    documents: List[str]
    top_k: int = 3


# ─── Helpers ──────────────────────────────────────────────────────────────────
def _format_matches(raw_matches) -> List[Dict]:
    """Normalise Pinecone match objects into a consistent dict."""
    out = []
    for m in raw_matches:
        meta = m.get("metadata", {}) if isinstance(m, dict) else getattr(m, "metadata", {}) or {}
        match_id = m.get("id", "") if isinstance(m, dict) else getattr(m, "id", "")
        score = m.get("score", 0.0) if isinstance(m, dict) else getattr(m, "score", 0.0)
        out.append({
            "id":       match_id,
            "score":    score,
            "text":     meta.get("text", ""),
            "metadata": meta,
        })
    return out


def _persist_bm25_stores() -> None:
    """Persist BM25 corpora and filenames so the knowledge base survives restarts."""
    os.makedirs(DATA_DIR, exist_ok=True)

    serialisable = {}
    for team_id, store in bm25_stores.items():
        serialisable[team_id] = {
            "corpus": store.get("corpus", []),
            "ids": store.get("ids", []),
            "filenames": sorted(store.get("filenames", set())),
        }

    tmp_path = f"{BM25_STORE_PATH}.tmp"
    with open(tmp_path, "w", encoding="utf-8") as handle:
        json.dump(serialisable, handle, ensure_ascii=True, indent=2)
    os.replace(tmp_path, BM25_STORE_PATH)


def _load_bm25_stores() -> None:
    """Restore persisted BM25 corpora and rebuild BM25 indexes at startup."""
    global bm25_stores

    if not os.path.exists(BM25_STORE_PATH):
        return

    try:
        with open(BM25_STORE_PATH, "r", encoding="utf-8") as handle:
            persisted = json.load(handle)
    except (OSError, json.JSONDecodeError) as err:
        print(f"  WARNING: could not load BM25 store from {BM25_STORE_PATH}: {err}")
        return

    restored = {}
    for team_id, store in persisted.items():
        corpus = list(store.get("corpus", []))
        ids = list(store.get("ids", []))
        filenames = set(store.get("filenames", []))

        restored_store = {
            "corpus": corpus,
            "ids": ids,
            "filenames": filenames,
            "bm25": BM25Okapi([text.lower().split() for text in corpus]) if corpus else None,
        }
        restored[team_id] = restored_store

    bm25_stores = restored
    if bm25_stores:
        print(f"  Restored BM25 knowledge base for {len(bm25_stores)} team(s) from disk")


def _reciprocal_rank_fusion(
    semantic_results: List[Dict],
    bm25_results: List[Dict],
    semantic_weight: float = 0.6,
    k: int = 60,
) -> List[Dict]:
    """
    Merge semantic and BM25 ranked lists using Reciprocal Rank Fusion.

    RRF score = Σ weight / (k + rank_i)

    Returns a list of merged result dicts sorted by descending RRF score.
    Each result dict has: id, text, metadata, rrf_score.
    """
    bm25_weight = 1.0 - semantic_weight

    # Build lookup: id → result dict
    all_docs: Dict[str, Dict] = {}
    scores:   Dict[str, float] = {}

    for rank, doc in enumerate(semantic_results, start=1):
        doc_id = doc["id"]
        all_docs[doc_id] = doc
        scores[doc_id] = scores.get(doc_id, 0.0) + semantic_weight / (k + rank)

    for rank, doc in enumerate(bm25_results, start=1):
        doc_id = doc["id"]
        all_docs[doc_id] = doc
        scores[doc_id] = scores.get(doc_id, 0.0) + bm25_weight / (k + rank)

    # Sort by fused score
    merged = sorted(all_docs.values(), key=lambda d: scores[d["id"]], reverse=True)
    for doc in merged:
        doc["rrf_score"] = scores[doc["id"]]
    return merged


# ─── Routes ───────────────────────────────────────────────────────────────────
@app.get("/health")
def health_check():
    return {
        "status":            "ok",
        "pinecone_connected": index is not None,
        "embed_model_loaded": embed_model is not None,
        "rerank_model_loaded": cross_encoder is not None,
        "bm25_teams":        list(bm25_stores.keys()),
    }


@app.post("/embed")
def embed_documents(req: EmbedRequest):
    """Embed documents, upsert to Pinecone, and update the in-memory BM25 index."""
    if index is None or embed_model is None:
        raise HTTPException(status_code=503, detail="Service not fully initialized")

    if not req.documents:
        return {"status": "success", "upserted_count": 0}

    texts     = [doc.text for doc in req.documents]
    ids       = [doc.id  for doc in req.documents]
    metadatas = [dict(doc.metadata) for doc in req.documents]

    # Stamp team_id on every metadata record
    for meta in metadatas:
        meta["team_id"] = req.team_id

    # ── Dense embeddings → Pinecone ──────────────────────────────────────────
    print(f"  Embedding {len(texts)} chunks for team '{req.team_id}'...")
    embeddings = embed_model.encode(texts, show_progress_bar=False).tolist()

    records = [
        {"id": doc_id, "values": emb, "metadata": meta}
        for doc_id, emb, meta in zip(ids, embeddings, metadatas)
    ]
    index.upsert(vectors=records, namespace=req.team_id)
    print(f"  Upserted {len(records)} vectors → Pinecone namespace '{req.team_id}'")

    # ── BM25 index update ─────────────────────────────────────────────────────
    store = bm25_stores.setdefault(
        req.team_id, {"corpus": [], "ids": [], "bm25": None, "filenames": set()}
    )
    store["corpus"].extend(texts)
    store["ids"].extend(ids)

    # Track unique filenames (stored in metadata by the Go ingestion service)
    for doc in req.documents:
        fname = doc.metadata.get("filename")
        if fname:
            store["filenames"].add(fname)

    # Rebuild BM25 over the full corpus
    tokenised = [text.lower().split() for text in store["corpus"]]
    store["bm25"] = BM25Okapi(tokenised)
    _persist_bm25_stores()
    print(f"  BM25 index rebuilt: {len(store['corpus'])} chunks | "
          f"{len(store['filenames'])} files in team '{req.team_id}'")

    return {"status": "success", "upserted_count": len(records)}


@app.post("/retrieve")
def retrieve_documents(req: QueryRequest):
    """Pure semantic retrieval from Pinecone (legacy — prefer /hybrid-retrieve)."""
    if index is None or embed_model is None:
        raise HTTPException(status_code=503, detail="Service not fully initialized")

    query_emb = embed_model.encode(req.query, show_progress_bar=False).tolist()
    results   = index.query(
        namespace=req.team_id,
        vector=query_emb,
        top_k=req.top_k,
        include_metadata=True,
    )
    return {"status": "success", "results": _format_matches(results.get("matches", []))}


@app.get("/list-docs")
def list_documents(team_id: str):
    """Phase 5 — List all document filenames ingested for a team.
    NOTE: named /list-docs (not /docs) because FastAPI reserves /docs for Swagger UI.
    """
    store = bm25_stores.get(team_id)
    if not store:
        return {"status": "success", "documents": [], "note": "No BM25 index found. Re-upload documents after restart."}
    docs = sorted(store.get("filenames", set()))
    return {"status": "success", "team_id": team_id, "documents": docs, "count": len(docs)}


@app.post("/hybrid-retrieve")
def hybrid_retrieve(req: HybridQueryRequest):
    """
    Phase 4 — Hybrid Retrieval:
      1. Dense semantic search via Pinecone (top_k * 2 candidates)
      2. BM25 keyword search over the in-memory corpus (top_k * 2 candidates)
      3. Reciprocal Rank Fusion to merge and re-rank both lists
      Returns the top_k fused results.
    """
    if index is None or embed_model is None:
        raise HTTPException(status_code=503, detail="Service not fully initialized")

    fetch_k = req.top_k * 2  # over-fetch before fusion

    # ── 1. Semantic search ───────────────────────────────────────────────────
    query_emb       = embed_model.encode(req.query, show_progress_bar=False).tolist()
    semantic_raw    = index.query(
        namespace=req.team_id,
        vector=query_emb,
        top_k=fetch_k,
        include_metadata=True,
    )
    semantic_results = _format_matches(semantic_raw.get("matches", []))

    # ── 2. BM25 keyword search ───────────────────────────────────────────────
    bm25_results: List[Dict] = []
    store = bm25_stores.get(req.team_id)

    if store and store.get("bm25") is not None:
        tokenised_query = req.query.lower().split()
        bm25_scores     = store["bm25"].get_scores(tokenised_query)

        # Build ranked list from BM25 scores
        scored = sorted(
            zip(store["ids"], store["corpus"], bm25_scores),
            key=lambda x: x[2],
            reverse=True,
        )
        for doc_id, text, score in scored[:fetch_k]:
            bm25_results.append({
                "id":       doc_id,
                "score":    float(score),
                "text":     text,
                "metadata": {"text": text, "team_id": req.team_id},
            })
        print(f"  BM25: {len(bm25_results)} candidates for team '{req.team_id}'")
    else:
        print(f"  BM25: no index for team '{req.team_id}' — using semantic-only")

    # ── 3. Reciprocal Rank Fusion ────────────────────────────────────────────
    if bm25_results:
        fused = _reciprocal_rank_fusion(
            semantic_results,
            bm25_results,
            semantic_weight=req.semantic_weight,
        )
    else:
        fused = semantic_results  # graceful fallback: pure semantic

    top_results = fused[: req.top_k]
    print(
        f"  Hybrid retrieve: {len(semantic_results)} semantic + "
        f"{len(bm25_results)} BM25 → {len(top_results)} fused results"
    )

    return {
        "status":  "success",
        "mode":    "hybrid" if bm25_results else "semantic-only",
        "results": top_results,
    }


@app.post("/rerank")
def rerank_documents(req: RerankRequest):
    """Cross-Encoder reranking of retrieved text passages."""
    if cross_encoder is None:
        raise HTTPException(status_code=503, detail="Reranking model not loaded")

    pairs  = [[req.query, doc] for doc in req.documents]
    scores = cross_encoder.predict(pairs)

    results = sorted(
        [{"document": doc, "score": float(score)} for doc, score in zip(req.documents, scores)],
        key=lambda x: x["score"],
        reverse=True,
    )
    return {"status": "success", "results": results[: req.top_k]}


if __name__ == "__main__":
    import uvicorn
    uvicorn.run("main:app", host="0.0.0.0", port=8000, reload=True)
