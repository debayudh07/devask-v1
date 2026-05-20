package cli

import (
	"fmt"
	"os"

	"backend/internal/config"
	"backend/internal/service"
	"backend/pkg/pyclient"
	"github.com/spf13/cobra"
)

var skillCmd = &cobra.Command{
	Use:   "skill",
	Short: "Generate a SKILL.md for the team's knowledge base",
}

var skillGenerateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate SKILL.md describing the team's knowledge base",
	Long:  `Creates a SKILL.md file that AI agents can read to understand what topics your team's knowledge base covers and how to query it.`,
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
		pythonURL := os.Getenv("PYTHON_AI_SERVICE_URL")
		if pythonURL == "" {
			pythonURL = "http://localhost:8000"
		}

		fmt.Printf("📄 Generating SKILL.md for team '%s'...\n", profile.TeamName)

		// Fetch ingested documents list
		pyClient := pyclient.New(pythonURL)
		docsResp, err := pyClient.ListDocs(profile.TeamID)
		docs := []string{}
		if err != nil {
			fmt.Printf("  ⚠ Could not fetch document list: %v\n  Generating SKILL.md without doc list.\n", err)
		} else {
			docs = docsResp.Documents
		}

		outputPath := "SKILL.md"
		gen := service.NewGeneratorService()
		if err := gen.GenerateSkill(profile, docs, outputPath); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("✅ SKILL.md generated → %s\n", outputPath)
		fmt.Printf("   Contains %d document(s) from team '%s'\n", len(docs), profile.TeamName)
		fmt.Println("\n💡 Tip: Commit SKILL.md to your repo so AI agents can discover your knowledge base.")
	},
}

func init() {
	skillCmd.AddCommand(skillGenerateCmd)
	rootCmd.AddCommand(skillCmd)
}
