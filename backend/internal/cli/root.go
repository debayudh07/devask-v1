package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "devask",
	Short: "Devask is an AI-powered knowledge assistant for engineering teams",
	Long: `A team-scoped CLI tool that ingests your scattered knowledge and makes it queryable via AI.
It acts as both a CLI for queries and a server for integrations (like MCP).`,
}

// Execute runs the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
