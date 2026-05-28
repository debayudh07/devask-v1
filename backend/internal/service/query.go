package service

import (
	"fmt"

	"github.com/debayudh07/devask/pkg/aiclient"
	"github.com/debayudh07/devask/pkg/pyclient"
)

// QueryService handles the full RAG pipeline: hybrid-retrieve → rerank → synthesize.
type QueryService struct {
	pyClient *pyclient.Client
	aiClient *aiclient.Client
}

// NewQueryService creates a new QueryService.
func NewQueryService(pyClient *pyclient.Client, aiClient *aiclient.Client) *QueryService {
	return &QueryService{pyClient: pyClient, aiClient: aiClient}
}

// QueryResult holds the final answer and the source chunks used.
type QueryResult struct {
	Answer       string   `json:"answer"`
	Sources      []string `json:"sources"`
	RetrievalMode string  `json:"retrieval_mode"` // "hybrid" | "semantic-only"
}

// Ask performs the full Phase 4 RAG pipeline for a given question and team:
//   Step 1 — Hybrid Retrieval (Semantic + BM25 via RRF)
//   Step 2 — Cross-Encoder Reranking (top 3 from top 10)
//   Step 3 — LLM Answer Synthesis
func (s *QueryService) Ask(teamID, question string) (*QueryResult, error) {
	// ── Step 1: Hybrid Retrieval ──────────────────────────────────────────────
	fmt.Printf("  [1/3] Hybrid retrieval for team '%s'...\n", teamID)

	hybridResp, err := s.pyClient.HybridRetrieve(teamID, question, 10, 0.6)
	var (
		matches       []pyclient.RetrieveMatch
		retrievalMode string
	)

	if err != nil {
		// Hybrid endpoint unavailable (e.g., old Python service) — fall back gracefully
		fmt.Printf("  ⚠ Hybrid retrieve failed (%v). Falling back to semantic-only.\n", err)
		semanticResp, sErr := s.pyClient.Retrieve(teamID, question, 10)
		if sErr != nil {
			return nil, fmt.Errorf("retrieve chunks: %w", sErr)
		}
		matches = semanticResp.Results
		retrievalMode = "semantic-only"
	} else {
		matches = hybridResp.Results
		retrievalMode = hybridResp.Mode
	}

	if len(matches) == 0 {
		return &QueryResult{
			Answer:       "I couldn't find any relevant information in the knowledge base for your question.",
			Sources:      []string{},
			RetrievalMode: retrievalMode,
		}, nil
	}

	fmt.Printf("  [1/3] ✓ %d chunks retrieved (mode: %s)\n", len(matches), retrievalMode)

	// ── Step 2: Extract chunk texts ───────────────────────────────────────────
	rawTexts := make([]string, 0, len(matches))
	for _, m := range matches {
		if m.Text != "" {
			rawTexts = append(rawTexts, m.Text)
		} else if text, ok := m.Metadata["text"].(string); ok && text != "" {
			rawTexts = append(rawTexts, text)
		}
	}

	if len(rawTexts) == 0 {
		return &QueryResult{
			Answer:       "Retrieved chunks contained no readable text. Please re-upload your documents.",
			Sources:      []string{},
			RetrievalMode: retrievalMode,
		}, nil
	}

	// ── Step 3: Cross-Encoder Reranking ──────────────────────────────────────
	topK := 3
	if len(rawTexts) < topK {
		topK = len(rawTexts)
	}

	fmt.Printf("  [2/3] Reranking %d chunks → top %d...\n", len(rawTexts), topK)

	rerankResp, err := s.pyClient.Rerank(question, rawTexts, topK)
	var topChunks []string

	if err != nil {
		fmt.Printf("  ⚠ Reranking failed (%v). Using top-%d from retrieval.\n", err, topK)
		topChunks = rawTexts[:topK]
	} else {
		topChunks = make([]string, len(rerankResp.Results))
		for i, r := range rerankResp.Results {
			topChunks[i] = r.Document
		}
		fmt.Printf("  [2/3] ✓ Reranked to %d chunks\n", len(topChunks))
	}

	// ── Step 4: LLM Answer Synthesis ─────────────────────────────────────────
	fmt.Printf("  [3/3] Synthesizing answer with %s...\n", s.aiClient.Model())

	answer, err := s.aiClient.SynthesizeAnswer(question, topChunks)
	if err != nil {
		return nil, fmt.Errorf("LLM synthesis failed: %w", err)
	}

	fmt.Printf("  [3/3] ✓ Answer generated\n")

	// ── Collect unique source filenames ───────────────────────────────────────
	seen := map[string]bool{}
	var sources []string
	for _, m := range matches {
		if fn, ok := m.Metadata["filename"].(string); ok && fn != "" && !seen[fn] {
			sources = append(sources, fn)
			seen[fn] = true
		}
	}

	return &QueryResult{
		Answer:       answer,
		Sources:      sources,
		RetrievalMode: retrievalMode,
	}, nil
}
