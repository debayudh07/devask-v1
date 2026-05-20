package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"

	"backend/internal/config"
	"github.com/spf13/cobra"
)

var uploadCmd = &cobra.Command{
	Use:   "upload [file]",
	Short: "Upload a document to the team's knowledge base",
	Long:  `Uploads a text, Markdown, code, or config file to the Devask backend. It will be chunked, embedded with BAAI/bge-base-en-v1.5, and stored in Pinecone under your team's namespace.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		filePath := args[0]

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

		// Open the file
		f, err := os.Open(filePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: cannot open file '%s': %v\n", filePath, err)
			os.Exit(1)
		}
		defer f.Close()

		filename := filepath.Base(filePath)
		fmt.Printf("📤 Uploading '%s' for team '%s' (%s)...\n", filename, profile.TeamName, profile.TeamID)

		// Build multipart form
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		part, err := writer.CreateFormFile("file", filename)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: create form file: %v\n", err)
			os.Exit(1)
		}
		if _, err = io.Copy(part, f); err != nil {
			fmt.Fprintf(os.Stderr, "Error: copy file content: %v\n", err)
			os.Exit(1)
		}
		writer.Close()

		// Send request to backend
		uploadURL := fmt.Sprintf("%s/upload?team_id=%s", serverURL, profile.TeamID)
		req, err := http.NewRequest(http.MethodPost, uploadURL, &body)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: create request: %v\n", err)
			os.Exit(1)
		}
		req.Header.Set("Content-Type", writer.FormDataContentType())

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
				Status        string `json:"status"`
				Message       string `json:"message"`
				UpsertedCount int    `json:"upserted_count"`
			}
			if err := json.Unmarshal(respBody, &result); err == nil {
				fmt.Printf("✅ %s\n", result.Message)
				fmt.Printf("   Embedded & upserted %d chunks to Pinecone.\n", result.UpsertedCount)
			} else {
				fmt.Println("✅ Upload successful.")
			}
		} else {
			var errResult struct {
				Error string `json:"error"`
			}
			json.Unmarshal(respBody, &errResult)
			fmt.Fprintf(os.Stderr, "❌ Upload failed (HTTP %d): %s\n", resp.StatusCode, errResult.Error)
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(uploadCmd)
}
