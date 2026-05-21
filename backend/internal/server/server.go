package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"backend/internal/handler"
	"backend/internal/service"
	"backend/pkg/aiclient"
	"backend/pkg/pyclient"
)

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "message": "Devask Backend is running!"})
}

// Start runs the HTTP server with all routes wired up.
func Start() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}

	pythonServiceURL := os.Getenv("PYTHON_AI_SERVICE_URL")
	if pythonServiceURL == "" {
		pythonServiceURL = "http://localhost:8000"
	}

	// Load LLM config from env (LLM_BASE_URL, LLM_MODEL, LLM_API_KEY)
	llmCfg := aiclient.DefaultConfig()
	if llmCfg.APIKey == "" {
		log.Fatal("FATAL: LLM_API_KEY (or OPENROUTER_API_KEY) environment variable is not set. Cannot start server.")
	}

	// --- Dependency wiring ---
	pyClient := pyclient.New(pythonServiceURL)
	aiClient := aiclient.New(llmCfg)

	ingestionSvc := service.NewIngestionService(pyClient)
	querySvc := service.NewQueryService(pyClient, aiClient)

	uploadHandler := handler.NewUploadHandler(ingestionSvc)
	askHandler    := handler.NewAskHandler(querySvc)
	docsHandler   := handler.NewDocsHandler(pyClient)
	llmHandler    := handler.NewLLMHandler(aiClient)

	// --- Routes ---
	mux := http.NewServeMux()
	mux.HandleFunc("/health", healthHandler)
	mux.Handle("/upload", uploadHandler)
	mux.Handle("/ask", askHandler)
	mux.Handle("/docs", docsHandler)
	mux.Handle("/llm/complete", llmHandler) // OSS LLM proxy — all AI completions route through here

	fmt.Printf("🚀 Devask Server starting on port %s...\n", port)
	fmt.Printf("   Python AI Service : %s\n", pythonServiceURL)
	fmt.Printf("   LLM Base URL      : %s\n", aiClient.BaseURL())
	fmt.Printf("   LLM Model         : %s\n", aiClient.Model())
	log.Fatal(http.ListenAndServe(":"+port, mux))
}
