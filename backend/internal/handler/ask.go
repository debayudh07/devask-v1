package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"backend/internal/service"
)

// AskHandler handles POST /ask requests.
type AskHandler struct {
	querySvc *service.QueryService
}

// NewAskHandler creates a new AskHandler.
func NewAskHandler(svc *service.QueryService) *AskHandler {
	return &AskHandler{querySvc: svc}
}

type askRequest struct {
	TeamID   string `json:"team_id"`
	Question string `json:"question"`
}

type askResponse struct {
	Status        string   `json:"status"`
	Answer        string   `json:"answer"`
	Sources       []string `json:"sources"`
	RetrievalMode string   `json:"retrieval_mode"` // "hybrid" | "semantic-only"
}

type askErrorResponse struct {
	Error string `json:"error"`
}

// ServeHTTP implements http.Handler.
func (h *AskHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(askErrorResponse{Error: "method not allowed"})
		return
	}

	var req askRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(askErrorResponse{Error: fmt.Sprintf("invalid JSON body: %v", err)})
		return
	}

	req.TeamID = strings.TrimSpace(req.TeamID)
	req.Question = strings.TrimSpace(req.Question)

	if req.TeamID == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(askErrorResponse{Error: "team_id is required"})
		return
	}
	if req.Question == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(askErrorResponse{Error: "question is required"})
		return
	}

	fmt.Printf("[Ask] Team: %s | Question: %s\n", req.TeamID, req.Question)

	result, err := h.querySvc.Ask(req.TeamID, req.Question)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(askErrorResponse{Error: fmt.Sprintf("query failed: %v", err)})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(askResponse{
		Status:        "success",
		Answer:        result.Answer,
		Sources:       result.Sources,
		RetrievalMode: result.RetrievalMode,
	})
}
