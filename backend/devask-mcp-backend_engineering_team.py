#!/usr/bin/env python3
"""
Devask MCP Server — backend engineering team
Team ID   : 22057c0b-35ce-468d-abba-cb6d4a94932b
Generated : 2026-05-21 01:46:18

Setup:
  pip install mcp httpx
  python devask-mcp-backend_engineering_team.py

Claude Desktop config (~/.config/claude/claude_desktop_config.json):
  {
    "mcpServers": {
      "devask-backend_engineering_team": {
        "command": "python",
        "args": ["devask-mcp-backend_engineering_team.py"]
      }
    }
  }
"""

import httpx
from mcp.server.fastmcp import FastMCP

# ── Config ────────────────────────────────────────────────────────────────────
DEVASK_SERVER_URL = "http://localhost:8081"
TEAM_ID           = "22057c0b-35ce-468d-abba-cb6d4a94932b"
TEAM_NAME         = "backend engineering team"
TECH_STACK        = "go, react, sql"

mcp = FastMCP("devask-backend_engineering_team")


# ── Tool: ask a question ──────────────────────────────────────────────────────
@mcp.tool()
def ask_backend_engineering_team(question: str) -> str:
    """
    Ask a question to the backend engineering team team's private knowledge base.
    Returns a cited answer synthesized by an LLM from the team's ingested documents.
    Tech stack: go, react, sql
    Use this when you need information about backend engineering team's systems, processes, or domain.
    """
    try:
        resp = httpx.post(
            f"{DEVASK_SERVER_URL}/ask",
            json={"team_id": TEAM_ID, "question": question},
            timeout=90.0,
        )
        resp.raise_for_status()
        data    = resp.json()
        answer  = data.get("answer", "No answer returned.")
        sources = data.get("sources", [])
        mode    = data.get("retrieval_mode", "unknown")
        result  = answer
        if sources:
            result += "\n\n📎 Sources: " + ", ".join(sources)
        result += f"\n🔀 Retrieval: {mode}"
        return result
    except httpx.HTTPStatusError as e:
        return f"Error: HTTP {e.response.status_code} from Devask server."
    except Exception as e:
        return f"Error querying knowledge base: {e}"


# ── Tool: list ingested documents ─────────────────────────────────────────────
@mcp.tool()
def list_backend_engineering_team_docs() -> str:
    """
    List all documents currently ingested in the backend engineering team knowledge base.
    """
    try:
        resp = httpx.get(
            f"{DEVASK_SERVER_URL}/docs",
            params={"team_id": TEAM_ID},
            timeout=30.0,
        )
        resp.raise_for_status()
        data = resp.json()
        docs = data.get("documents", [])
        if not docs:
            return "No documents ingested yet. Run: devask upload <file>"
        lines = [f"📚 {TEAM_NAME} knowledge base ({len(docs)} document(s)):", ""]
        lines += [f"  • {d}" for d in docs]
        return "\n".join(lines)
    except Exception as e:
        return f"Error listing documents: {e}"


if __name__ == "__main__":
    mcp.run()
