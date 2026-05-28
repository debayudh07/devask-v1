package cli

import (
	"github.com/debayudh07/devask/internal/server"

	"github.com/spf13/cobra"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the Devask API server",
	Long:  `Starts the REST API server to handle requests from the CLI and other clients like MCP.`,
	Run: func(cmd *cobra.Command, args []string) {
		server.Start()
	},
}

func init() {
	rootCmd.AddCommand(serveCmd)
}
