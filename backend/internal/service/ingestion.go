package service

import (
	"fmt"
	"io"
	"strings"
	"unicode"

	"github.com/debayudh07/devask/pkg/pyclient"
)

// IngestionService handles document upload and chunking logic.
type IngestionService struct {
	pyClient *pyclient.Client
}

// NewIngestionService creates a new IngestionService.
func NewIngestionService(pyClient *pyclient.Client) *IngestionService {
	return &IngestionService{pyClient: pyClient}
}

// IngestDocument chunks the provided text and sends it to the Python AI service for embedding.
// teamID is the Pinecone namespace. filename is used in metadata.
func (s *IngestionService) IngestDocument(teamID, filename string, r io.Reader) (int, error) {
	// Read the full content
	raw, err := io.ReadAll(r)
	if err != nil {
		return 0, fmt.Errorf("read document content: %w", err)
	}
	content := string(raw)

	// Chunk the content into overlapping segments
	chunks := chunkText(content, 512, 64) // 512-token chunks with 64-token overlap
	if len(chunks) == 0 {
		return 0, fmt.Errorf("document is empty or could not be chunked")
	}

	// Build document records
	docs := make([]pyclient.Document, len(chunks))
	for i, chunk := range chunks {
		docs[i] = pyclient.Document{
			ID:   fmt.Sprintf("%s-%s-chunk-%d", teamID, sanitizeID(filename), i),
			Text: chunk,
			Metadata: map[string]interface{}{
				"filename":    filename,
				"chunk_index": i,
				"team_id":     teamID,
				"text":        chunk, // store text in metadata for retrieval
			},
		}
	}

	fmt.Printf("  → Chunked into %d segments. Sending to AI service for embedding...\n", len(docs))

	resp, err := s.pyClient.Embed(teamID, docs)
	if err != nil {
		return 0, fmt.Errorf("embed documents: %w", err)
	}

	return resp.UpsertedCount, nil
}

// chunkText splits text into overlapping chunks of approximately chunkSize words,
// with an overlap of overlapSize words between consecutive chunks.
func chunkText(text string, chunkSize, overlapSize int) []string {
	words := strings.FieldsFunc(text, func(r rune) bool {
		return unicode.IsSpace(r)
	})

	if len(words) == 0 {
		return nil
	}

	var chunks []string
	start := 0
	for start < len(words) {
		end := start + chunkSize
		if end > len(words) {
			end = len(words)
		}
		chunk := strings.Join(words[start:end], " ")
		if strings.TrimSpace(chunk) != "" {
			chunks = append(chunks, chunk)
		}
		if end == len(words) {
			break
		}
		start = end - overlapSize
		if start < 0 {
			start = 0
		}
	}
	return chunks
}

// sanitizeID replaces characters that are not safe for use in Pinecone vector IDs.
func sanitizeID(s string) string {
	var sb strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' {
			sb.WriteRune(r)
		} else {
			sb.WriteRune('-')
		}
	}
	return sb.String()
}
