# Devask — Team-Scoped AI Knowledge Assistant

> **Devask** turns your team's scattered documentation into a private, queryable knowledge base — accessible via CLI, REST API, and Claude / Cursor / Antigravity through MCP.

---

## How it works

```
Your docs (Markdown, PDF, text)
        │
        ▼  devask upload <file>
┌─────────────────────────┐
│   Go Backend (CLI/API)  │  :8081
│   devask serve          │──── POST /ask ──────────────┐
└─────────┬───────────────┘                             │
          │ chunks + embed requests                      │
          ▼                                             ▼
┌─────────────────────────┐              ┌──────────────────────┐
│ Python AI Microservice  │  :8000       │  LLM  (gpt-oss:120b) │
│ BAAI/bge-base-en-v1.5   │              │  via Ollama API       │
│ BM25 + Pinecone + RRF   │              └──────────────────────┘
│ CrossEncoder reranker   │
└─────────────────────────┘
          │
          ▼
   Pinecone (cloud vector DB)
   namespace-isolated per team
```

**Pipeline (when you ask a question):**

1. **Hybrid Retrieval** — Pinecone semantic search + BM25 keyword search, fused with Reciprocal Rank Fusion
2. **Cross-Encoder Reranking** — `ms-marco-MiniLM-L-6-v2` selects the top 3 most relevant chunks
3. **LLM Synthesis** — `gpt-oss:120b` (via Ollama) produces a cited answer; all OSS model traffic routes through the devask server's `/llm/complete` proxy
4. **SKILL.md generation** — AI-crafted developer skill files (grounded in real KB content) for Antigravity, Claude, Cursor

---

## Repository layout

```
go-project-fullstack/
├── backend/                    # Go CLI + API server
│   ├── cmd/devask/             # main.go — CLI entry point
│   ├── internal/
│   │   ├── cli/                # cobra commands (ask, upload, serve, skill, mcp, docs, init)
│   │   ├── handler/            # HTTP handlers (ask, upload, docs, llm proxy)
│   │   ├── server/             # HTTP server wiring + route registration
│   │   ├── service/            # Business logic (ingestion, query, generator)
│   │   └── config/             # TeamProfile config (~/.devask/config.json)
│   ├── pkg/
│   │   ├── aiclient/           # OpenAI-compatible LLM HTTP client
│   │   └── pyclient/           # Python AI microservice HTTP client
│   ├── Makefile
│   └── Dockerfile
├── python-ai-service/          # FastAPI — embedding, BM25, retrieval, reranking
│   ├── main.py
│   ├── requirements.txt
│   └── Dockerfile
├── docker-compose.yml          # Full-stack deployment
├── Caddyfile                   # Optional reverse proxy (HTTPS)
└── .env.example                # All required environment variables
```

---

## Prerequisites

| Tool | Minimum version | Notes |
|---|---|---|
| Go | 1.21+ | [go.dev/dl](https://go.dev/dl) |
| Python | 3.10+ | Anaconda or system Python |
| Pinecone account | — | Free tier works; [pinecone.io](https://www.pinecone.io) |
| Ollama API key | — | [ollama.com](https://ollama.com) — model `gpt-oss:120b` |
| Docker + Compose | v2+ | Only for containerised deployment |

---

## Quickstart (local, no Docker)

### 1 — Clone and configure

```bash
git clone https://github.com/your-org/devask.git
cd devask

# Copy env template and fill in your keys
cp .env.example backend/.env
# edit backend/.env:
#   LLM_API_KEY=<your-ollama-api-key>
#   PINECONE_API_KEY=<your-pinecone-api-key>
```

### 2 — Start the Python AI microservice

```bash
cd python-ai-service

# Create and activate a virtual environment
python -m venv .venv
# Windows:
.venv\Scripts\activate
# macOS / Linux:
source .venv/bin/activate

pip install -r requirements.txt
python main.py
# Listening on http://localhost:8000
```

### 3 — Build and start the Go backend

```bash
cd backend

# Build the CLI binary
make build          # produces devask.exe (Windows) / devask (Linux/macOS)

# Start the API server
devask serve        # or: make run
# Listening on http://localhost:8081
```

### 4 — Initialise your team

```bash
devask init
# Prompts for: team name, description, tech stack
# Creates ~/.devask/config.json with a unique team UUID
```

### 5 — Upload documents

```bash
devask upload path/to/your/runbook.md
devask upload path/to/architecture.pdf
devask docs     # list all ingested documents
```

### 6 — Ask questions

```bash
devask ask "How does our authentication middleware work?"
devask ask "What are the retry policies for the payment service?"
```

---

## Docker deployment

```bash
# Copy and fill in env vars
cp .env.example .env
# edit .env with your keys

# Start Python AI service + Go backend
docker compose up -d

# Optional: add Caddy for HTTPS termination
docker compose --profile caddy up -d
```

Services:
- Python AI microservice → `http://localhost:8000`
- Devask API server → `http://localhost:8081`
- Caddy (optional) → `http://localhost:80` / `https://localhost:443`

---

## CLI reference

```
devask [command]

Commands:
  init        Initialise a new team profile
  upload      Upload and ingest a document into the knowledge base
  ask         Ask a question to the knowledge base
  docs        List all ingested documents
  serve       Start the Devask API server
  skill       Generate AI-crafted SKILL.md developer skill files
  mcp         Generate a Python MCP server for Claude Desktop / Cursor

Flags:
  -h, --help  help for any command
```

### SKILL.md generation

Devask can generate developer skill files — equivalent to `.cursorrules` or `CLAUDE.md` — grounded in real content retrieved from your knowledge base:

```bash
# Full knowledge base SKILL.md
devask skill generate

# Focused on a specific sub-domain
devask skill generate --skill database    # → SKILL-database.md
devask skill generate --skill api         # → SKILL-api.md
devask skill generate --skill auth        # → SKILL-auth.md
devask skill generate --skill deployment  # → SKILL-deployment.md
```

Each SKILL.md contains: Architecture Overview, Coding Conventions, File Structure, Implementation Patterns, DO/DON'T Rules, Integration Points, and an Extension Guide — all extracted from your actual ingested documents.

### MCP integration (Claude Desktop / Cursor / Antigravity)

```bash
# Generate the MCP server script
devask mcp generate

# Install dependencies
pip install mcp httpx

# Add to your Claude Desktop / Cursor config:
# ~/.config/claude/claude_desktop_config.json
{
  "mcpServers": {
    "devask-<your-team>": {
      "command": "python",
      "args": ["/path/to/devask-mcp-<your-team>.py"]
    }
  }
}
```

Available MCP tools after setup:
- `ask_<team>(question)` — Query your knowledge base
- `list_<team>_docs()` — List all ingested documents

---

## REST API

All requests go to `http://localhost:8081`.

| Method | Path | Description |
|---|---|---|
| `GET` | `/health` | Server health check |
| `POST` | `/upload` | Ingest a document (multipart/form-data) |
| `POST` | `/ask` | Query the knowledge base |
| `GET` | `/docs?team_id=<id>` | List ingested documents |
| `POST` | `/llm/complete` | OSS LLM proxy — forwards to gpt-oss via Ollama |

**POST /ask**
```json
// Request
{ "team_id": "<uuid>", "question": "How does X work?" }

// Response
{
  "status": "success",
  "answer": "Based on Chunk 1...",
  "sources": ["runbook.md", "architecture.md"],
  "retrieval_mode": "hybrid"
}
```

**POST /llm/complete** *(internal — used by `devask skill generate`)*
```json
// Request
{ "messages": [{"role": "system", "content": "..."}, {"role": "user", "content": "..."}] }

// Response
{ "content": "...", "model": "gpt-oss:120b" }
```

---

## Environment variables

| Variable | Default | Required | Description |
|---|---|---|---|
| `LLM_BASE_URL` | `https://ollama.com/v1` | ✅ | OpenAI-compatible LLM API base URL |
| `LLM_MODEL` | `gpt-oss:120b` | ✅ | Model identifier |
| `LLM_API_KEY` | — | ✅ | Bearer token for the LLM API |
| `PINECONE_API_KEY` | — | ✅ | Pinecone API key |
| `PINECONE_INDEX_NAME` | `devask-knowledge` | — | Pinecone index name |
| `PORT` | `8081` | — | Go server port |
| `PYTHON_AI_SERVICE_URL` | `http://localhost:8000` | — | Python microservice URL |
| `DEVASK_SERVER_URL` | `http://localhost:8081` | — | Used by CLI to proxy LLM calls through server |

---

## Development

```bash
# Backend — hot-reload with gow
cd backend
make run-server

# Backend — standard run
make run

# Backend — build binary
make build

# Backend — quick ask (server must be running)
make ask Q="How does our deployment pipeline work?"

# Backend — quick upload
make upload F="../devask-context.md"

# Python service — run with auto-reload
cd python-ai-service
uvicorn main:app --reload --port 8000
```

---

## Tech stack

| Layer | Technology |
|---|---|
| CLI & API server | Go 1.21+, Cobra, standard `net/http` |
| AI microservice | Python 3.10+, FastAPI, Uvicorn |
| Embeddings | `BAAI/bge-base-en-v1.5` (768-dim, via sentence-transformers) |
| Reranker | `cross-encoder/ms-marco-MiniLM-L-6-v2` |
| Vector database | Pinecone (serverless, AWS us-east-1) |
| Keyword search | BM25 (`rank_bm25`), in-memory with JSON persistence |
| Retrieval fusion | Reciprocal Rank Fusion (RRF) |
| LLM | `gpt-oss:120b` via Ollama (OpenAI-compatible API) |
| Containerisation | Docker + Docker Compose, optional Caddy reverse proxy |

---

## Licence

MIT — see [LICENSE](LICENSE) for details.
