package pyclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client is an HTTP client for the Python AI microservice.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// New creates a new Python AI microservice client.
func New(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 120 * time.Second, // embedding can take a while
		},
	}
}

// Document is a chunk of text with metadata to be embedded.
type Document struct {
	ID       string                 `json:"id"`
	Text     string                 `json:"text"`
	Metadata map[string]interface{} `json:"metadata"`
}

// EmbedRequest is the payload sent to POST /embed.
type EmbedRequest struct {
	TeamID    string     `json:"team_id"`
	Documents []Document `json:"documents"`
}

// EmbedResponse is the response from POST /embed.
type EmbedResponse struct {
	Status        string `json:"status"`
	UpsertedCount int    `json:"upserted_count"`
}

// RetrieveRequest is the payload sent to POST /retrieve.
type RetrieveRequest struct {
	TeamID string `json:"team_id"`
	Query  string `json:"query"`
	TopK   int    `json:"top_k"`
}

// HybridRetrieveRequest is the payload sent to POST /hybrid-retrieve.
type HybridRetrieveRequest struct {
	TeamID          string  `json:"team_id"`
	Query           string  `json:"query"`
	TopK            int     `json:"top_k"`
	SemanticWeight  float64 `json:"semantic_weight"` // 0.0–1.0; default 0.6
}

// RetrieveMatch is a single retrieved chunk.
type RetrieveMatch struct {
	ID       string                 `json:"id"`
	Score    float64                `json:"score"`
	RRFScore float64                `json:"rrf_score"`    // populated in hybrid mode
	Text     string                 `json:"text"`         // chunk text (stored in Pinecone metadata at upsert time)
	Metadata map[string]interface{} `json:"metadata"`
}

// RetrieveResponse is the response from POST /retrieve.
type RetrieveResponse struct {
	Status  string          `json:"status"`
	Results []RetrieveMatch `json:"results"`
}

// HybridRetrieveResponse is the response from POST /hybrid-retrieve.
type HybridRetrieveResponse struct {
	Status  string          `json:"status"`
	Mode    string          `json:"mode"`    // "hybrid" or "semantic-only"
	Results []RetrieveMatch `json:"results"`
}

// RerankRequest is the payload sent to POST /rerank.
type RerankRequest struct {
	Query     string   `json:"query"`
	Documents []string `json:"documents"`
	TopK      int      `json:"top_k"`
}

// RerankResult is a single reranked document.
type RerankResult struct {
	Document string  `json:"document"`
	Score    float64 `json:"score"`
}

// RerankResponse is the response from POST /rerank.
type RerankResponse struct {
	Status  string         `json:"status"`
	Results []RerankResult `json:"results"`
}

// Embed sends documents to the Python service for chunking, embedding, and upserting to Pinecone.
func (c *Client) Embed(teamID string, docs []Document) (*EmbedResponse, error) {
	payload := EmbedRequest{TeamID: teamID, Documents: docs}
	return doPost[EmbedResponse](c, "/embed", payload)
}

// Retrieve queries the Python service for semantically similar chunks.
func (c *Client) Retrieve(teamID, query string, topK int) (*RetrieveResponse, error) {
	payload := RetrieveRequest{TeamID: teamID, Query: query, TopK: topK}
	return doPost[RetrieveResponse](c, "/retrieve", payload)
}

// HybridRetrieve queries the Python service using combined semantic + BM25 keyword
// search, fused via Reciprocal Rank Fusion. Falls back to semantic-only if the
// team's BM25 index hasn't been populated yet.
func (c *Client) HybridRetrieve(teamID, query string, topK int, semanticWeight float64) (*HybridRetrieveResponse, error) {
	if semanticWeight <= 0 {
		semanticWeight = 0.6 // default: 60% semantic, 40% BM25
	}
	payload := HybridRetrieveRequest{
		TeamID:         teamID,
		Query:          query,
		TopK:           topK,
		SemanticWeight: semanticWeight,
	}
	return doPost[HybridRetrieveResponse](c, "/hybrid-retrieve", payload)
}

// Rerank reranks a set of text passages using a cross-encoder.
func (c *Client) Rerank(query string, docs []string, topK int) (*RerankResponse, error) {
	payload := RerankRequest{Query: query, Documents: docs, TopK: topK}
	return doPost[RerankResponse](c, "/rerank", payload)
}

// HealthCheck pings the Python service to ensure it's up.
func (c *Client) HealthCheck() error {
	resp, err := c.httpClient.Get(c.baseURL + "/health")
	if err != nil {
		return fmt.Errorf("python AI service unreachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("python AI service health check failed: %s", resp.Status)
	}
	return nil
}

// DocsResponse is the response from GET /docs.
type DocsResponse struct {
	Status    string   `json:"status"`
	TeamID    string   `json:"team_id"`
	Documents []string `json:"documents"`
	Count     int      `json:"count"`
	Note      string   `json:"note,omitempty"`
}

// ListDocs returns the list of ingested document filenames for a team.
// Calls GET /list-docs (not /docs — that is FastAPI's reserved Swagger UI path).
func (c *Client) ListDocs(teamID string) (*DocsResponse, error) {
	resp, err := c.httpClient.Get(fmt.Sprintf("%s/list-docs?team_id=%s", c.baseURL, teamID))
	if err != nil {
		return nil, fmt.Errorf("list docs request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read list docs response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("python service returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	var result DocsResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("unmarshal list docs response: %w (body: %s)", err, string(body))
	}
	return &result, nil
}

// doPost is a generic helper for JSON POST requests.
func doPost[T any](c *Client, path string, payload interface{}) (*T, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, c.baseURL+path, bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request to %s: %w", path, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("python AI service returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result T
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response from %s: %w", path, err)
	}

	return &result, nil
}
