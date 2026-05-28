package handler

import (
	"encoding/json"
	"net/http"

	"github.com/debayudh07/devask/pkg/pyclient"
)

// DocsHandler handles GET /docs requests.
type DocsHandler struct {
	pyClient *pyclient.Client
}

// NewDocsHandler creates a new DocsHandler.
func NewDocsHandler(pyClient *pyclient.Client) *DocsHandler {
	return &DocsHandler{pyClient: pyClient}
}

// ServeHTTP implements http.Handler.
func (h *DocsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "method not allowed"})
		return
	}

	teamID := r.URL.Query().Get("team_id")
	if teamID == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "team_id query parameter is required"})
		return
	}

	result, err := h.pyClient.ListDocs(teamID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(result)
}
