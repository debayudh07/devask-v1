package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/debayudh07/devask/internal/config"
	"github.com/debayudh07/devask/internal/service"
	"github.com/spf13/cobra"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Generate and manage the MCP server for Claude integration",
}

var mcpGenerateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate a Python MCP server script for Claude Desktop",
	Long: `Generates a self-contained Python MCP server script that exposes your team's
knowledge base as tools inside Claude Desktop, Claude Code, and any MCP client.

The generated script exposes two tools:
  • ask_<teamname>(question)    — Ask a question to the knowledge base
  • list_<teamname>_docs()      — List all ingested documents

Usage after generation:
  pip install mcp httpx
  python devask-mcp-<teamname>.py`,
	Run: func(cmd *cobra.Command, args []string) {
		profile, err := config.LoadProfile()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: run 'devask init' first.\n%v\n", err)
			os.Exit(1)
		}

		serverURL := os.Getenv("DEVASK_SERVER_URL")
		if serverURL == "" {
			serverURL = "http://localhost:8081"
		}

		slug := service.ToSlug(profile.TeamName)
		outputFile := fmt.Sprintf("devask-mcp-%s.py", slug)

		// Resolve absolute path for Claude Desktop config snippet
		absPath, err := filepath.Abs(outputFile)
		if err != nil {
			absPath = outputFile
		}

		fmt.Printf("⚙️  Generating MCP server for team '%s'...\n", profile.TeamName)

		gen := service.NewGeneratorService()
		if err := gen.GenerateMCP(profile, serverURL, outputFile); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("✅ MCP server generated → %s\n\n", outputFile)
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		fmt.Println("Next steps:")
		fmt.Println("")
		fmt.Println("  1. Install dependencies:")
		fmt.Println("       pip install mcp httpx")
		fmt.Println("")
		fmt.Printf("  2. Add to Claude Desktop config\n")
		fmt.Printf("     (~/.config/claude/claude_desktop_config.json):\n\n")
		fmt.Printf(`     {
       "mcpServers": {
         "devask-%s": {
           "command": "python",
           "args": ["%s"]
         }
       }
     }%s`, slug, absPath, "\n\n")
		fmt.Println("  3. Restart Claude Desktop — your knowledge base tools will appear.")
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	},
}

func init() {
	mcpCmd.AddCommand(mcpGenerateCmd)
	rootCmd.AddCommand(mcpCmd)
}
