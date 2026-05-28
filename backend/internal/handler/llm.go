package handler

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/debayudh07/devask/pkg/aiclient"
)

// LLMHandler proxies chat-completion requests through the Devask server to the
// configured LLM backend (gpt-oss via Ollama / OpenRouter / any OpenAI-compat API).
//
// POST /llm/complete
//
//	Request  { "messages": [{"role":"system","content":"..."},{"role":"user","content":"..."}] }
//	Response { "content": "...", "model": "gpt-oss:120b" }
type LLMHandler struct {
	aiClient *aiclient.Client
}

// NewLLMHandler creates a new LLMHandler.
func NewLLMHandler(ai *aiclient.Client) *LLMHandler {
	return &LLMHandler{aiClient: ai}
}

type llmCompleteRequest struct {
	Messages []aiclient.Message `json:"messages"`
}

type llmCompleteResponse struct {
	Content string `json:"content"`
	Model   string `json:"model"`
}

type llmErrorResponse struct {
	Error string `json:"error"`
}

// ServeHTTP implements http.Handler.
func (h *LLMHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(llmErrorResponse{Error: "method not allowed"})
		return
	}

	var req llmCompleteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(llmErrorResponse{Error: fmt.Sprintf("invalid JSON body: %v", err)})
		return
	}

	if len(req.Messages) == 0 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(llmErrorResponse{Error: "messages array is required and must not be empty"})
		return
	}

	fmt.Printf("[LLM] Proxying completion request (%d messages) → %s\n",
		len(req.Messages), h.aiClient.Model())

	content, err := h.aiClient.Complete(req.Messages)
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		json.NewEncoder(w).Encode(llmErrorResponse{Error: fmt.Sprintf("LLM request failed: %v", err)})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(llmCompleteResponse{
		Content: content,
		Model:   h.aiClient.Model(),
	})
}
