package cli

import (
	"fmt"
	"os"

	"backend/internal/config"
	"backend/pkg/pyclient"
	"github.com/spf13/cobra"
)

var docsCmd = &cobra.Command{
	Use:   "docs",
	Short: "List all documents ingested into the team's knowledge base",
	Long:  `Shows all document filenames currently indexed in your team's Pinecone namespace.`,
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

		pyClient := pyclient.New(pythonURL)
		resp, err := pyClient.ListDocs(profile.TeamID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ Error listing documents: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("📚 Knowledge base for team '%s' (%s)\n\n", profile.TeamName, profile.TeamID)

		if len(resp.Documents) == 0 {
			fmt.Println("  No documents ingested yet.")
			fmt.Println("  Upload a document with: devask upload <file>")
			if resp.Note != "" {
				fmt.Printf("\n  ℹ️  Note: %s\n", resp.Note)
			}
			return
		}

		fmt.Printf("  %d document(s) indexed:\n\n", resp.Count)
		for _, doc := range resp.Documents {
			fmt.Printf("   • %s\n", doc)
		}
	},
}

func init() {
	rootCmd.AddCommand(docsCmd)
}
