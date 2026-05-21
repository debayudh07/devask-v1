package cli

import (
	"fmt"
	"os"
	"strings"

	"backend/internal/config"
	"backend/internal/service"
	"backend/pkg/aiclient"
	"backend/pkg/pyclient"
	"github.com/spf13/cobra"
)

// skillFocusFlag holds the value of the --skill flag (e.g. "database", "api").
var skillFocusFlag string

var skillCmd = &cobra.Command{
	Use:   "skill",
	Short: "Generate a SKILL.md for the team's knowledge base",
}

var skillGenerateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate SKILL.md describing the team's knowledge base",
	Long: `Creates a SKILL.md that AI agents (Antigravity, Claude, Cursor) can read
to understand what your team's knowledge base covers and how to query it.

The file is grounded in REAL content retrieved from your knowledge base using
hybrid search (Pinecone semantic + BM25), then crafted by gpt-oss following
the Antigravity/Claude/Cursor skill-file convention.

Use --skill to scope the output to a specific sub-domain:

  devask skill generate                    # full knowledge base SKILL.md
  devask skill generate --skill database   # database-only SKILL.md
  devask skill generate --skill api        # REST API / gRPC layer only
  devask skill generate --skill auth       # authentication & authorisation
  devask skill generate --skill deployment # CI/CD and infrastructure

If the LLM or Python service is unreachable, a static fallback is used so
the command always succeeds.`,
	Run: func(cmd *cobra.Command, args []string) {
		profile, err := config.LoadProfile()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: run 'devask init' first.\n%v\n", err)
			os.Exit(1)
		}

		pythonURL := os.Getenv("PYTHON_AI_SERVICE_URL")
		if pythonURL == "" {
			pythonURL = "http://localhost:8000"
		}

		focus := strings.TrimSpace(skillFocusFlag)

		if focus != "" {
			fmt.Printf("📄 Generating SKILL.md for team '%s' (focus: %s)...\n", profile.TeamName, focus)
		} else {
			fmt.Printf("📄 Generating SKILL.md for team '%s' (full knowledge base)...\n", profile.TeamName)
		}

		pyClient := pyclient.New(pythonURL)

		// ── 1. Fetch ingested document filenames ──────────────────────────────
		docsResp, err := pyClient.ListDocs(profile.TeamID)
		docs := []string{}
		if err != nil {
			fmt.Printf("  ⚠ Could not fetch document list: %v\n  Continuing without doc list.\n", err)
		} else {
			docs = docsResp.Documents
		}

		// ── 2. Retrieve real KB chunks for grounding ──────────────────────────
		// Run several queries so gpt-oss gets a representative cross-section of
		// the KB (or a focused slice when --skill is set).
		chunks := retrieveContextChunks(pyClient, profile.TeamID, profile, focus)

		// ── Wire AI client — route LLM calls via the devask server ────────────
		aiCfg := aiclient.DefaultConfig()
		ai := aiclient.New(aiCfg)

		// Route all OSS model traffic through the central devask server.
		// Falls back to direct LLM call if DEVASK_SERVER_URL is not set.
		serverURL := os.Getenv("DEVASK_SERVER_URL")
		if serverURL == "" {
			serverURL = "http://localhost:8081"
		}

		outputPath := "SKILL.md"
		if focus != "" {
			outputPath = fmt.Sprintf("SKILL-%s.md", service.ToSlug(focus))
		}

		// ── Generate ───────────────────────────────────────────────────────────
		gen := service.NewGeneratorServiceViaServer(ai, serverURL)
		opts := service.SkillOptions{
			ContextChunks: chunks,
			SkillFocus:    focus,
		}
		if err := gen.GenerateSkill(profile, docs, opts, outputPath); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("✅ SKILL.md generated → %s\n", outputPath)
		fmt.Printf("   %d KB chunk(s) used · %d document(s) in knowledge base\n",
			len(chunks), len(docs))
		if focus != "" {
			fmt.Printf("   Skill scope: %s\n", focus)
		}
		fmt.Printf("   LLM routed via: %s/llm/complete\n", serverURL)
		fmt.Println("\n💡 Tip: Commit this file to your repo so AI agents can discover your knowledge base.")
	},
}

// retrieveContextChunks pulls representative text chunks from the Python backend.
//
// When focus is empty:  runs 4 broad queries covering architecture, APIs,
// database, and processes — giving gpt-oss a wide cross-section of the KB.
//
// When focus is set:    runs queries tightly scoped to that sub-domain plus one
// broad fallback so there is always some grounding context.
//
// Chunks are deduplicated by ID and capped at 20 to keep the LLM prompt
// within a reasonable token budget.
func retrieveContextChunks(py *pyclient.Client, teamID string, profile *config.TeamProfile, focus string) []string {
	queries := buildRetrievalQueries(profile, focus)

	seen := map[string]bool{}
	var chunks []string
	const maxChunks = 20
	const chunksPerQuery = 5

	for _, q := range queries {
		if len(chunks) >= maxChunks {
			break
		}
		resp, err := py.HybridRetrieve(teamID, q, chunksPerQuery, 0.6)
		if err != nil {
			fmt.Printf("  ⚠ Retrieval failed for query %q: %v\n", q, err)
			continue
		}
		for _, m := range resp.Results {
			if len(chunks) >= maxChunks {
				break
			}
			// Prefer the Text field; fall back to metadata["text"]
			text := m.Text
			if text == "" {
				if t, ok := m.Metadata["text"].(string); ok {
					text = t
				}
			}
			if text == "" || seen[m.ID] {
				continue
			}
			seen[m.ID] = true
			chunks = append(chunks, text)
		}
	}

	fmt.Printf("  📦 Retrieved %d unique KB chunk(s) across %d queries\n", len(chunks), len(queries))
	return chunks
}

// buildRetrievalQueries returns the set of search queries used to sample the KB.
// When a focus is provided the queries are tightly scoped to that sub-domain.
func buildRetrievalQueries(profile *config.TeamProfile, focus string) []string {
	if focus == "" {
		// Broad queries — sample a wide cross-section of the KB
		name := profile.TeamName
		return []string{
			fmt.Sprintf("%s system architecture and design", name),
			fmt.Sprintf("%s REST API endpoints and data models", name),
			fmt.Sprintf("%s database schema migrations and queries", name),
			fmt.Sprintf("%s deployment CI/CD pipelines and infrastructure", name),
			fmt.Sprintf("%s authentication authorization and security", name),
			fmt.Sprintf("%s error handling debugging and troubleshooting", name),
		}
	}

	// Focused queries — drill into the specific sub-domain
	name := profile.TeamName
	f := focus
	return []string{
		fmt.Sprintf("%s %s design and architecture", name, f),
		fmt.Sprintf("%s %s patterns and best practices", name, f),
		fmt.Sprintf("%s %s schema configuration and setup", name, f),
		fmt.Sprintf("%s %s errors debugging and troubleshooting", name, f),
		fmt.Sprintf("%s %s examples and usage", name, f),
	}
}

func init() {
	skillGenerateCmd.Flags().StringVarP(
		&skillFocusFlag, "skill", "s", "",
		`Scope SKILL.md to a specific sub-domain (e.g. "database", "api", "auth", "deployment")`,
	)
	skillCmd.AddCommand(skillGenerateCmd)
	rootCmd.AddCommand(skillCmd)
}
