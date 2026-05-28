package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/debayudh07/devask/internal/config"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a new team profile",
	Long:  `Creates a team profile with a tech stack declaration and unique team ID stored in ~/.devask/config.json.`,
	Run: func(cmd *cobra.Command, args []string) {
		reader := bufio.NewReader(os.Stdin)

		fmt.Print("Enter Team Name (e.g., Platform Engineering): ")
		name, _ := reader.ReadString('\n')
		name = strings.TrimSpace(name)

		fmt.Print("Enter Team Description: ")
		desc, _ := reader.ReadString('\n')
		desc = strings.TrimSpace(desc)

		fmt.Print("Enter Tech Stack (comma separated, e.g., Go, React, Postgres): ")
		stackStr, _ := reader.ReadString('\n')
		stackStr = strings.TrimSpace(stackStr)
		
		var stack []string
		for _, s := range strings.Split(stackStr, ",") {
			stack = append(stack, strings.TrimSpace(s))
		}

		if err := config.InitProfile(name, desc, stack); err != nil {
			fmt.Printf("Error initializing profile: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
}
