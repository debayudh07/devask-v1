The core idea is building a **team-scoped CLI tool called `devask`** that gives any engineering team their own private, AI-powered knowledge assistant — one they fully control, can feed with their own documents, and can deploy as a standard MCP server that plugs directly into Claude Desktop, Claude Code, or any other MCP-compatible client.

---

## What the platform actually does

At its heart, devask solves a problem every engineering team faces: institutional knowledge is scattered everywhere. Architecture decisions live in Confluence pages nobody reads. Onboarding docs are outdated PDFs. The real answers to "how do we deploy to production?" or "why did we choose Kafka over RabbitMQ?" live in Slack threads from six months ago or in the heads of senior engineers who are always in meetings. New developers waste days hunting for answers that exist somewhere but are impossible to find. devask fixes this by giving teams a single command-line tool that ingests all of that scattered knowledge and makes it queryable in plain English.

---

## The team profile system

Every team starts by running `devask init`, which creates a team profile — a named workspace with a tech stack declaration, a description, and a unique team ID stored in a local config file at `~/.devask/config.json`. This profile is the anchor for everything else. A company might have multiple teams (platform engineering, data engineering, mobile) each with their own completely separate knowledge base, their own generated skill files, and their own MCP server. Developers switch between team contexts with `devask team switch <team_id>`, so a person who works across teams can query the right knowledge base without confusion. The team ID maps to a dedicated ChromaDB collection on the backend, so data is always namespaced — one team's docs never bleed into another team's answers.

---

## Document ingestion and the vector knowledge base

Once a team profile exists, members start feeding it documents with `devask upload <file>`. The tool accepts plain text files, Markdown, and PDFs. When a document is uploaded, the Go API backend receives it and forwards it to a Python AI microservice that extracts its raw text, splits it into 400-word overlapping chunks (so context is never cut off at an arbitrary boundary), runs each chunk through a sentence embedding model called `BAAI/bge-base-en-v1.5`, and stores the resulting vectors plus metadata (source filename, chunk index, team ID) in ChromaDB using upsert so re-uploading a document never creates duplicates. Over time the knowledge base grows to contain runbooks, architecture decision records, API documentation, onboarding guides, coding standards, postmortem reports — whatever the team feeds it. The more documents ingested, the better and more specific the answers become.

---

## Querying with RAG

When a developer runs `devask ask "how do we rotate the Postgres credentials?"`, the CLI sends that question to the Go API backend, which coordinates with the Python AI microservice to run a hybrid retrieval — it simultaneously does a semantic vector search (finding chunks that are conceptually similar to the question) and a BM25 keyword search (finding chunks that contain the exact words). The results from both searches are merged, deduplicated, and then passed through a cross-encoder reranking model that scores each candidate chunk for how relevant it actually is to the specific question. The top three or four chunks are assembled into a context block and sent to Claude along with a system prompt that instructs it to answer only from the provided context, cite source filenames, and say clearly if the answer isn't available rather than hallucinating. The answer comes back to the terminal with source citations — so the developer knows exactly which document to read if they want more detail. The entire round trip takes a few seconds.

---

## Generating a SKILL.md

This is where it gets genuinely novel. Once a team has uploaded enough documents, they can run `devask skill generate`, which triggers the Go backend to sample up to 40 chunks from the knowledge base (via the Python microservice), send them to Claude, and ask it to analyse what this team's knowledge base actually covers — what topics appear, what the team's conventions are, what kinds of questions this knowledge base is well-positioned to answer. Claude synthesises that analysis into a structured SKILL.md file, which is a machine-readable description of the skill's purpose, scope, trigger conditions, and formatting expectations. This is the same format used in Anthropic's internal skill system and in frameworks like Antigravity. The generated file declares things like "trigger this skill when a developer asks about deployment procedures or authentication flows for the platform team's stack" — giving any AI agent that reads it precise instructions for when and how to use this team's knowledge base. Developers download the SKILL.md with `devask skill download` and can drop it into their AI agent framework, their Claude Project instructions, or their own tool's skill directory.

---

## Generating and deploying the MCP server

`devask mcp generate` takes the team profile and auto-generates a complete, runnable Python MCP server script. The generated server exposes two tools to any MCP client: `ask_<teamname>` which takes a plain-English question and returns a cited answer from the knowledge base, and `list_<teamname>_docs` which returns a summary of what documents have been ingested. The server is just a Python file that talks to the Go API backend over HTTP. Developers run `devask mcp download` to save the script locally, install two dependencies (`mcp` and `httpx`), add a small JSON snippet to their Claude Desktop config file, and from that point on Claude can natively call into the team's knowledge base as a tool — without the developer doing anything special. It just appears as a connected tool in Claude's context. For teams that want to host the MCP server so the whole team can share it without each person running it locally, the included Docker Compose file spins up the entire backend (Go API + Python AI Service + ChromaDB + uploaded documents) as a containerised service. Adding the optional Caddy reverse proxy profile gives it HTTPS, making it accessible as a proper hosted MCP endpoint that any team member's Claude client can point at — effectively a private, team-owned knowledge API running on whatever infrastructure the team prefers, whether that's a VM on Railway, a Fly.io machine, or an internal server behind a VPN.

---

## The full workflow end to end

The beautiful thing about how these pieces fit together is that a team can go from zero to a fully functional AI knowledge assistant in about fifteen minutes. They run `devask init`, answer three questions about their team name and stack, upload five or six documents, run `devask ask` a few times to verify the answers make sense, then run `devask skill generate` and `devask mcp generate` back to back. At that point they have a local CLI that answers questions, a SKILL.md that any AI agent can read to understand this team's domain, and a hosted MCP server that makes the knowledge base available as a first-class tool inside Claude. Every new document they upload automatically enriches all three — the CLI answers get better, the skill file can be regenerated to reflect new topics, and the MCP server immediately starts returning answers that include the new content because it talks to the same backend. The knowledge base becomes a living artifact that grows with the team rather than a document graveyard that immediately goes stale.