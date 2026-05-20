# Evaluation and Implementation Blueprint for `devask`

## Idea Evaluation

**Strengths:**
- **Highly Solvable Problem:** Knowledge scattering is a massive pain point. RAG is the perfect solution.
- **Go's Dual Nature:** Using Go for both the CLI and the main API backend is a brilliant move. A single Go binary can act as the CLI (`devask ask`) and the server (`devask serve`). This drastically simplifies distribution and deployment.
- **Python Microservice:** Offloading just the ML heavy lifting (embedding, reranking, file parsing) to Python keeps the Go backend fast and memory-efficient while fully utilizing Python's rich AI ecosystem.

## Technical Decisions

1. **Communication Protocol:** The Go backend will communicate with the Python AI microservice over standard **REST (HTTP)**. This is simpler to implement, easy to debug, and perfectly adequate for local or containerized microservice communication without the overhead of gRPC protobuf definitions.
2. **Database:** We will use **SQLite** embedded directly within the Go backend to manage team profiles, document metadata, and settings. This keeps the deployment incredibly simple (no external Postgres dependency needed) and fits perfectly with the dual CLI/Server nature of the tool.

---

## Blueprint for Implementation

### Architecture Overview
1. **The Go Binary (`devask`)**:
   - **CLI Mode**: Handles commands like `devask init`, `devask upload`, `devask ask`.
   - **Server Mode**: Running `devask serve` spins up the main API backend (REST).
   - **Responsibilities**: Orchestrating workflows, managing SQLite/JSON metadata, handling user sessions, generating `SKILL.md` and MCP server scripts, and formatting final LLM calls to Claude.
2. **Python AI Microservice**:
   - A lightweight FastAPI (or gRPC) service running purely as an engine for the Go backend.
   - **Responsibilities**: Document chunking, Sentence embeddings (`BAAI/bge-base-en-v1.5`), Pinecone DB interactions, BM25 indexing, and Cross-Encoder reranking.
3. **LLM Integration**: The Go backend will call Claude (Anthropic API) for final answer synthesis.

### Phased Execution Plan

#### Phase 1: Go CLI & Server Foundation
- Set up the Go project using `cobra` for CLI commands.
- Implement the dual nature: `devask` CLI commands vs `devask serve` (using `net/http` or `Gin`).
- Implement `devask init` and local config management (`~/.devask/config.json`).
- Implement the API routes on the Go server that the CLI will consume.

#### Phase 2: Python AI Microservice setup
- Create the Python FastAPI microservice.
- Set up Pinecone DB and a BM25 store inside the Python service.
- Create internal endpoints: `/embed`, `/retrieve`, `/rerank`.

#### Phase 3: Ingestion Pipeline (Go <-> Python) ✅ COMPLETE
- ✅ `devask upload <file>` CLI — reads team profile, multipart POST to Go server
- ✅ Go server `/upload` handler — receives file, calls ingestion service
- ✅ Ingestion service — chunks text (512-word chunks, 64-word overlap), calls Python `/embed`
- ✅ Python microservice — embeds with `BAAI/bge-base-en-v1.5`, upserts to Pinecone (namespace = team_id)
- ✅ `devask ask <question>` CLI — retrieve → rerank → LLM synthesis via OpenRouter
- ✅ Go server `/ask` handler — full RAG pipeline
- ✅ Query service — Pinecone retrieval (top 10) → Cross-Encoder rerank (top 3) → LLM synthesis
- ✅ OpenRouter integration — `meta-llama/llama-3.2-3b-instruct:free` model
  - Management key used to provision inference key: `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`
  - Key stored in `backend/.env` as `OPENROUTER_API_KEY`
- ✅ New packages: `backend/pkg/pyclient`, `backend/pkg/aiclient`
- ✅ New services: `backend/internal/service/ingestion.go`, `backend/internal/service/query.go`
- ✅ New handlers: `backend/internal/handler/upload.go`, `backend/internal/handler/ask.go`

#### Phase 4: Hybrid Retrieval & Querying ✅ COMPLETE
- ✅ **BM25 keyword index** — built in Python per-team on every `/embed` call (`rank_bm25.BM25Okapi`)
- ✅ **CrossEncoder moved to lifespan** — loaded once at startup, reused across all `/rerank` requests
- ✅ **`POST /hybrid-retrieve`** — new Python endpoint:
  - Semantic search: Pinecone top_k×2 candidates
  - BM25 keyword search: in-memory corpus top_k×2 candidates
  - **Reciprocal Rank Fusion (RRF)** merges both lists (default: 60% semantic, 40% BM25)
  - Returns `mode: "hybrid"` or `"semantic-only"` (graceful fallback if BM25 not populated)
- ✅ **`pyclient.HybridRetrieve`** — Go method for the new endpoint with configurable `semantic_weight`
- ✅ **`QueryService.Ask` upgraded** — uses hybrid retrieval as primary path; falls back to semantic-only if hybrid endpoint is unavailable
- ✅ **Pipeline steps** logged: `[1/3] Hybrid retrieve → [2/3] Rerank → [3/3] LLM synthesis`
- ✅ **CLI output** shows retrieval mode: 🔀 hybrid or 🔍 semantic-only

#### Phase 5: Skill & MCP Generation
- Implement `devask skill generate`.
- Implement `devask mcp generate` (Go backend templates a Python script that talks to the Go API).
- Docker Compose setup: `devask` (server mode) + `python-ai-service` + `Caddy`. (Note: Pinecone is cloud-hosted, so no local vector DB container is needed).

## Verification Plan
1. **Unit/Integration Tests**: Mock the Python microservice to test the Go server and CLI in isolation.
2. **End-to-End Walkthrough**: Run the entire 15-minute workflow manually (Init -> Upload Docs -> Ask -> Generate Skill -> Generate MCP) with both the Go Server and Python Microservice running.
