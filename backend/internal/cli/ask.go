package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/debayudh07/devask/internal/config"
	"github.com/spf13/cobra"
)

var askCmd = &cobra.Command{
	Use:   "ask [question...]",
	Short: "Ask a question to the team's knowledge base",
	Long:  `Queries the devask backend with a plain English question. It retrieves relevant documents from Pinecone, reranks them, and synthesizes a final answer using deepseek/deepseek-v4-flash via OpenRouter.`,
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		question := strings.Join(args, " ")

		// Load team profile to get team_id
		profile, err := config.LoadProfile()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: could not load team profile. Run 'devask init' first.\n%v\n", err)
			os.Exit(1)
		}

		// Determine server URL
		serverURL := os.Getenv("DEVASK_SERVER_URL")
		if serverURL == "" {
			serverURL = "http://localhost:8081"
		}

		fmt.Printf("🔍 Asking: \"%s\"\n", question)
		fmt.Printf("   Team: %s (%s)\n\n", profile.TeamName, profile.TeamID)

		// Build request payload
		payload := map[string]string{
			"team_id":  profile.TeamID,
			"question": question,
		}
		body, err := json.Marshal(payload)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: marshal request: %v\n", err)
			os.Exit(1)
		}

		// Send to backend server
		req, err := http.NewRequest(http.MethodPost, serverURL+"/ask", bytes.NewBuffer(body))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: create request: %v\n", err)
			os.Exit(1)
		}
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: could not reach Devask server at %s\nMake sure the server is running: devask serve\n%v\n", serverURL, err)
			os.Exit(1)
		}
		defer resp.Body.Close()

		respBody, _ := io.ReadAll(resp.Body)

		if resp.StatusCode == http.StatusOK {
			var result struct {
				Status        string   `json:"status"`
				Answer        string   `json:"answer"`
				Sources       []string `json:"sources"`
				RetrievalMode string   `json:"retrieval_mode"`
			}
			if err := json.Unmarshal(respBody, &result); err != nil {
				fmt.Fprintf(os.Stderr, "Error: parse response: %v\n", err)
				os.Exit(1)
			}

			modeIcon := "🔀"
			if result.RetrievalMode == "semantic-only" {
				modeIcon = "🔍"
			}

			fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			fmt.Printf("🤖 Answer (DeepSeek V4 Flash via Ollama):\n\n%s\n", result.Answer)
			fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			fmt.Printf("%s Retrieval mode: %s\n", modeIcon, result.RetrievalMode)

			if len(result.Sources) > 0 {
				fmt.Printf("\n📎 Sources:\n")
				for _, src := range result.Sources {
					fmt.Printf("   • %s\n", src)
				}
			}
		} else {
			var errResult struct {
				Error string `json:"error"`
			}
			json.Unmarshal(respBody, &errResult)
			fmt.Fprintf(os.Stderr, "❌ Query failed (HTTP %d): %s\n", resp.StatusCode, errResult.Error)
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(askCmd)
}
