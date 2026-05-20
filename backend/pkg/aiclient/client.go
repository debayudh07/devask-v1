package aiclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// Config holds all configuration for the AI client.
// Values are read from environment variables, with sensible defaults.
type Config struct {
	// BaseURL is the OpenAI-compatible API base (e.g. https://ollama.com/v1 or https://openrouter.ai/api/v1)
	BaseURL string
	// Model is the model identifier (e.g. gpt-oss:120b)
	Model string
	// APIKey is the Bearer token for the API
	APIKey string
}

// DefaultConfig reads config from environment variables.
//
//	LLM_BASE_URL  — defaults to https://ollama.com/v1
//	LLM_MODEL     — defaults to gpt-oss:120b
//	LLM_API_KEY   — falls back to OPENROUTER_API_KEY if unset
func DefaultConfig() Config {
	baseURL := os.Getenv("LLM_BASE_URL")
	if baseURL == "" {
		baseURL = "https://ollama.com/v1"
	}
	// strip trailing slash for clean URL joining
	baseURL = strings.TrimRight(baseURL, "/")

	model := os.Getenv("LLM_MODEL")
	if model == "" {
		model = "gpt-oss:120b"
	}

	apiKey := os.Getenv("LLM_API_KEY")
	if apiKey == "" {
		// backward-compat fallback
		apiKey = os.Getenv("OPENROUTER_API_KEY")
	}

	return Config{BaseURL: baseURL, Model: model, APIKey: apiKey}
}

// Client is an OpenAI-compatible HTTP client (works with Ollama, OpenRouter, etc.)
type Client struct {
	cfg        Config
	httpClient *http.Client
}

// New creates a new AI client using the provided config.
func New(cfg Config) *Client {
	return &Client{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: 120 * time.Second, // larger models can be slow
		},
	}
}

// Message represents a single chat message.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatRequest is the OpenAI-compatible payload.
type chatRequest struct {
	Model     string    `json:"model"`
	Messages  []Message `json:"messages"`
	MaxTokens int       `json:"max_tokens,omitempty"`
	Stream    bool      `json:"stream"`
}

// chatResponse is the OpenAI-compatible response envelope.
type chatResponse struct {
	Choices []struct {
		Message struct {
			Content   string `json:"content"`
			Reasoning string `json:"reasoning"` // some models (e.g. gpt-oss) return reasoning here
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string      `json:"message"`
		Code    interface{} `json:"code"` // can be int or string depending on provider
	} `json:"error,omitempty"`
}

// Complete sends messages to the LLM and returns the assistant's reply.
func (c *Client) Complete(messages []Message) (string, error) {
	payload := chatRequest{
		Model:     c.cfg.Model,
		Messages:  messages,
		MaxTokens: 4096, // large enough for reasoning models
		Stream:    false,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal chat request: %w", err)
	}

	endpoint := c.cfg.BaseURL + "/chat/completions"
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewBuffer(body))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	}
	// Optional metadata headers (ignored by providers that don't support them)
	req.Header.Set("HTTP-Referer", "https://github.com/devask-cli")
	req.Header.Set("X-Title", "Devask Knowledge Assistant")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("call LLM API (%s): %w", endpoint, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read LLM response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("LLM API returned HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var result chatResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("unmarshal LLM response: %w (body: %s)", err, string(respBody))
	}

	if result.Error != nil {
		return "", fmt.Errorf("LLM API error: %v", result.Error.Message)
	}

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("LLM returned no choices (body: %s)", string(respBody))
	}

	// Prefer content; fall back to reasoning field (used by thinking models like gpt-oss)
	content := result.Choices[0].Message.Content
	if content == "" {
		content = result.Choices[0].Message.Reasoning
	}
	if content == "" {
		return "", fmt.Errorf("LLM returned empty content and empty reasoning (body: %s)", string(respBody))
	}

	return content, nil
}

// Model returns the configured model name (useful for logging).
func (c *Client) Model() string {
	return c.cfg.Model
}

// BaseURL returns the configured base URL (useful for logging).
func (c *Client) BaseURL() string {
	return c.cfg.BaseURL
}

// SynthesizeAnswer builds a RAG prompt from context chunks and a question, then calls the LLM.
func (c *Client) SynthesizeAnswer(question string, contextChunks []string) (string, error) {
	contextBlock := ""
	for i, chunk := range contextChunks {
		contextBlock += fmt.Sprintf("[Chunk %d]\n%s\n\n", i+1, chunk)
	}

	systemPrompt := `You are Devask, a precise and helpful AI knowledge assistant for engineering teams.
Your job is to answer questions based ONLY on the provided context chunks from the team's knowledge base.
If the answer is not in the context, say "I couldn't find relevant information in the knowledge base."
Always cite which chunk(s) your answer comes from (e.g., "Based on Chunk 1...").
Be concise and technical. Avoid hallucination.`

	userPrompt := fmt.Sprintf(`CONTEXT FROM KNOWLEDGE BASE:
%s
---
QUESTION: %s

Answer based strictly on the context above.`, contextBlock, question)

	messages := []Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}

	return c.Complete(messages)
}
