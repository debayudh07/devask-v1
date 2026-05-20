# Devask Backend & Database Architecture

## Project Overview

Devask is a team-scoped AI knowledge assistant built as a Go CLI + API server, backed by a Python AI microservice for all ML operations.
The binary is dual-mode: `devask serve` starts the REST API, and all other subcommands (`init`, `upload`, `ask`, `docs`, `skill`, `mcp`) act as a CLI client.

---

## Tech Stack

| Layer | Technology | Purpose |
|---|---|---|
| CLI + API Server | Go 1.21 (Cobra + net/http) | CLI commands, REST API, orchestration |
| AI Microservice | Python 3.11 (FastAPI + Uvicorn) | Embedding, BM25, reranking |
| Vector Database | Pinecone (cloud, serverless) | Dense vector storage + semantic search |
| Keyword Index | BM25 (rank_bm25, in-memory) | Sparse keyword search per team |
| Embedding Model | BAAI/bge-base-en-v1.5 (768-dim) | Dense text embeddings |
| Reranking Model | cross-encoder/ms-marco-MiniLM-L-6-v2 | Cross-encoder reranking |
| LLM | gpt-oss:120b via Ollama API | Answer synthesis |
| Team Config | JSON file (~/.devask/config.json) | Local team profile storage |

---

## Go Backend Directory Structure

```
backend/
├── cmd/devask/main.go              # Entry point — loads .env, runs CLI
├── internal/
│   ├── cli/
│   │   ├── root.go                 # Cobra root command
│   │   ├── init.go                 # devask init — create team profile
│   │   ├── upload.go               # devask upload <file>
│   │   ├── ask.go                  # devask ask <question>
│   │   ├── docs.go                 # devask docs — list ingested files
│   │   ├── skill.go                # devask skill generate
│   │   ├── mcp.go                  # devask mcp generate
│   │   └── serve.go                # devask serve — start API server
│   ├── config/
│   │   └── config.go               # TeamProfile struct, Load/Init helpers
│   ├── handler/
│   │   ├── upload.go               # POST /upload
│   │   ├── ask.go                  # POST /ask
│   │   └── docs.go                 # GET /docs
│   ├── service/
│   │   ├── ingestion.go            # Chunk text → call Python /embed
│   │   ├── query.go                # Hybrid retrieve → rerank → LLM
│   │   └── generator.go            # Generate SKILL.md and MCP .py script
│   └── server/
│       └── server.go               # HTTP server, route registration
└── pkg/
    ├── pyclient/client.go          # Go HTTP client for Python AI service
    └── aiclient/client.go          # Go HTTP client for Ollama/OpenRouter LLM
```

---

## Team Profile — Local Config Database

The primary "database" for team metadata is a **JSON file** at `~/.devask/config.json`.
There is no external SQL database. SQLite is planned for a future phase.

### Schema (config.json)
```json
{
  "team_id":    "22057c0b-35ce-468d-abba-cb6d4a94932b",
  "team_name":  "Platform Engineering",
  "description": "Manages internal infrastructure and developer tooling",
  "tech_stack": ["Go", "Kubernetes", "Postgres", "Terraform"]
}
```

### Go struct (backend/internal/config/config.go)
```go
type TeamProfile struct {
    TeamID      string   `json:"team_id"`
    TeamName    string   `json:"team_name"`
    Description string   `json:"description"`
    TechStack   []string `json:"tech_stack"`
}
```

### Key functions
- `config.InitProfile(name, desc, stack)` — creates config.json with a new UUID team_id
- `config.LoadProfile()` — reads and parses config.json
- `config.GetConfigPath()` — returns `~/.devask/config.json`

---

## Vector Database — Pinecone

### Index Configuration
- **Index name:** `devask-knowledge` (configurable via `PINECONE_INDEX_NAME`)
- **Dimension:** 768 (matches BAAI/bge-base-en-v1.5 output)
- **Metric:** cosine similarity
- **Cloud:** AWS us-east-1 (serverless)
- **Namespace per team:** each team's vectors are isolated in `namespace = team_id`

### Vector Record Schema
Each uploaded document chunk becomes one Pinecone vector:
```json
{
  "id": "<team_id>-<sanitized_filename>-chunk-<index>",
  "values": [0.123, -0.456, ...],  // 768-dim embedding
  "metadata": {
    "team_id":    "22057c0b-35ce-468d-abba-cb6d4a94932b",
    "filename":   "runbook-deploy.md",
    "chunk_index": 3,
    "text":       "To deploy to production: create a GitHub release..."
  }
}
```

### Chunking Strategy (ingestion.go)
- **Chunk size:** 512 words
- **Overlap:** 64 words between consecutive chunks
- **Method:** whitespace tokenization (`strings.FieldsFunc`)
- **ID format:** `<team_id>-<sanitized_filename>-chunk-<N>`

---

## BM25 Keyword Index — In-Memory

The Python service maintains a per-team BM25 index in memory using `rank_bm25.BM25Okapi`.

### Structure (python-ai-service/main.py)
```python
bm25_stores: Dict[str, Dict] = {
    "<team_id>": {
        "bm25":      BM25Okapi(...),   # tokenized corpus index
        "corpus":    ["chunk text 1", "chunk text 2", ...],
        "ids":       ["<vector_id_1>", "<vector_id_2>", ...],
        "filenames": {"runbook.md", "architecture.md"}  # set of unique filenames
    }
}
```

### Important: Persistence
The BM25 index is **in-memory only**. It is rebuilt from scratch on each `/embed` call.
After a Python service restart, the BM25 index will be empty until documents are re-uploaded.
Pinecone data persists across restarts (it is cloud-hosted).

---

## REST API Endpoints

### Go Server (default: :8081)

| Method | Path | Description |
|---|---|---|
| GET | `/health` | Health check — returns `{"status":"ok"}` |
| POST | `/upload?team_id=<id>` | Upload a file (multipart/form-data, field: `file`) |
| POST | `/ask` | Ask a question — JSON body: `{"team_id":"...","question":"..."}` |
| GET | `/docs?team_id=<id>` | List ingested document filenames for a team |

### Python AI Service (default: :8000)

| Method | Path | Description |
|---|---|---|
| GET | `/health` | Health check with model/Pinecone status |
| POST | `/embed` | Embed documents and upsert to Pinecone + BM25 |
| POST | `/retrieve` | Pure semantic search (legacy) |
| POST | `/hybrid-retrieve` | Semantic + BM25 + RRF fusion (Phase 4) |
| POST | `/rerank` | Cross-encoder reranking |
| GET | `/list-docs?team_id=<id>` | List filenames from BM25 store |

**Note:** `/list-docs` not `/docs` — FastAPI reserves `/docs` for Swagger UI.

---

## Query Pipeline (devask ask)

```
CLI: devask ask "How do I deploy?"
  │
  ▼
POST /ask  →  Go server (handler/ask.go)
  │
  ├─[1/3] POST /hybrid-retrieve  →  Python AI service
  │        Semantic search:  Pinecone query (top 20 candidates)
  │        BM25 search:      in-memory keyword score (top 20 candidates)
  │        Fusion:           Reciprocal Rank Fusion (semantic 60%, BM25 40%)
  │        Returns:          top 10 merged chunks with rrf_score
  │
  ├─[2/3] POST /rerank  →  Python AI service
  │        Input:  top 10 chunk texts
  │        Model:  cross-encoder/ms-marco-MiniLM-L-6-v2
  │        Output: top 3 most relevant chunks (sorted by cross-encoder score)
  │
  └─[3/3] POST /chat/completions  →  Ollama API (gpt-oss:120b)
           System prompt:  Devask RAG instructions
           User prompt:    [Chunk 1] ... [Chunk 2] ... [Chunk 3] + QUESTION
           Returns:        cited answer referencing chunk numbers
```

### Ask Response Schema
```json
{
  "status": "success",
  "answer": "Based on Chunk 1, to deploy to production...",
  "sources": ["runbook-deploy.md"],
  "retrieval_mode": "hybrid"
}
```

---

## Upload Pipeline (devask upload)

```
CLI: devask upload runbook.md
  │
  ├─ Load ~/.devask/config.json → get team_id
  ├─ Read file content
  ├─ Build multipart/form-data payload
  │
  ▼
POST /upload?team_id=<id>  →  Go server (handler/upload.go)
  │  Allowed extensions: .txt .md .go .py .js .ts .json .yaml .yml .sh .toml .rs
  │
  ├─ service/ingestion.go: chunkText(content, 512, 64)  →  []string chunks
  │
  └─ POST /embed  →  Python AI service
       For each chunk:
         - Compute embedding: BAAI/bge-base-en-v1.5 → float[768]
         - Upsert to Pinecone (namespace = team_id)
         - Add to BM25 corpus
         - Track filename in bm25_stores[team_id]["filenames"]
```

---

## Environment Variables

### backend/.env
```
PORT=8081
PYTHON_AI_SERVICE_URL=http://localhost:8000
LLM_BASE_URL=https://ollama.com/v1
LLM_MODEL=gpt-oss:120b
LLM_API_KEY=<your-ollama-key>
```

### python-ai-service/.env
```
PINECONE_API_KEY=<your-pinecone-key>
PINECONE_INDEX_NAME=devask-knowledge
PORT=8000
```

### CLI env overrides
```
DEVASK_SERVER_URL=http://localhost:8081   # override Go server URL for CLI commands
PYTHON_AI_SERVICE_URL=http://localhost:8000  # override Python service URL
```

---

## LLM Client Configuration (pkg/aiclient/client.go)

The LLM client is provider-agnostic (OpenAI-compatible API format):
```go
type Config struct {
    BaseURL string  // LLM_BASE_URL env var (default: https://ollama.com/v1)
    Model   string  // LLM_MODEL env var (default: gpt-oss:120b)
    APIKey  string  // LLM_API_KEY env var (fallback: OPENROUTER_API_KEY)
}
```

**Supported providers:** Ollama (hosted or local), OpenRouter, any OpenAI-compatible endpoint.

The client handles:
- `gpt-oss:120b` returns answer in `reasoning` field (not `content`) — automatically detected and used as fallback
- `max_tokens: 4096` to accommodate reasoning model output
- Headers: `HTTP-Referer`, `X-Title` for OpenRouter compatibility

---

## MCP Server (Generated)

`devask mcp generate` produces a Python script (`devask-mcp-<teamslug>.py`) exposing two tools:

| Tool | Description |
|---|---|
| `ask_<teamslug>(question: str)` | Calls `POST /ask` on the Go server |
| `list_<teamslug>_docs()` | Calls `GET /docs?team_id=<id>` on the Go server |

Dependencies: `pip install mcp httpx`

Claude Desktop config:
```json
{
  "mcpServers": {
    "devask-platform_engineering": {
      "command": "python",
      "args": ["/path/to/devask-mcp-platform_engineering.py"]
    }
  }
}
```

---

## Docker Compose Services

```yaml
python-ai-service:   # :8000 — embedding, BM25, reranking
devask-server:       # :8081 — Go API server (depends on python-ai-service)
caddy:               # :80/:443 — HTTPS reverse proxy (optional profile)
```

Start: `docker compose up --build`
With HTTPS: `docker compose --profile caddy up --build`

---

## Known Limitations & Future Work

| Limitation | Impact | Planned Fix |
|---|---|---|
| BM25 index is in-memory | Lost on Python service restart | Persist to disk (pickle or SQLite) |
| No authentication on REST API | Anyone with network access can query | Add JWT or API key middleware |
| SQLite not yet implemented | Team metadata only in local JSON | Phase 6: SQLite for multi-team server |
| Single Pinecone index | All teams share one index (namespace-isolated) | Per-team index option for isolation |
| File types limited | Only text-based files | Add PDF parsing (pdfminer/pypdf) |
