package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"

	"github.com/debayudh07/devask/internal/service"
)

// UploadHandler handles POST /upload requests.
type UploadHandler struct {
	ingestionSvc *service.IngestionService
}

// NewUploadHandler creates a new UploadHandler.
func NewUploadHandler(svc *service.IngestionService) *UploadHandler {
	return &UploadHandler{ingestionSvc: svc}
}

type uploadResponse struct {
	Status        string `json:"status"`
	Message       string `json:"message"`
	UpsertedCount int    `json:"upserted_count"`
}

type uploadErrorResponse struct {
	Error string `json:"error"`
}

// ServeHTTP implements http.Handler.
func (h *UploadHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(uploadErrorResponse{Error: "method not allowed"})
		return
	}

	// Get team_id from query params
	teamID := r.URL.Query().Get("team_id")
	if teamID == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(uploadErrorResponse{Error: "team_id query parameter is required"})
		return
	}

	// Parse multipart form with max 50MB
	if err := r.ParseMultipartForm(50 << 20); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(uploadErrorResponse{Error: fmt.Sprintf("parse multipart form: %v", err)})
		return
	}

	file, fileHeader, err := r.FormFile("file")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(uploadErrorResponse{Error: fmt.Sprintf("get form file: %v", err)})
		return
	}
	defer file.Close()

	filename := filepath.Base(fileHeader.Filename)
	fmt.Printf("[Upload] Team: %s | File: %s | Size: %d bytes\n", teamID, filename, fileHeader.Size)

	// Validate file type — accept .txt, .md, .go, .py, .js, .ts, .json, .yaml, .yml
	ext := filepath.Ext(filename)
	allowedExts := map[string]bool{
		".txt": true, ".md": true, ".go": true, ".py": true,
		".js": true, ".ts": true, ".json": true, ".yaml": true, ".yml": true,
		".sh": true, ".env": true, ".toml": true, ".rs": true,
	}
	if !allowedExts[ext] {
		w.WriteHeader(http.StatusUnsupportedMediaType)
		json.NewEncoder(w).Encode(uploadErrorResponse{Error: fmt.Sprintf("unsupported file type: %s. Allowed: txt, md, go, py, js, ts, json, yaml", ext)})
		return
	}

	// Read file content and pass to ingestion service
	count, err := h.ingestionSvc.IngestDocument(teamID, filename, io.Reader(file))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(uploadErrorResponse{Error: fmt.Sprintf("ingest document: %v", err)})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(uploadResponse{
		Status:        "success",
		Message:       fmt.Sprintf("Document '%s' ingested successfully.", filename),
		UpsertedCount: count,
	})
}
