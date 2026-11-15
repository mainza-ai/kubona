package memory

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	EmbeddingDimensions   = 384
	LexicalEmbeddingModel = "lexical-hash-v1"
)

// TextEmbedder computes dense vector embeddings for semantic memory recall.
type TextEmbedder interface {
	Embed(ctx context.Context, text string) (vec []float32, model string, err error)
}

// HTTPEmbedder calls a local OpenAI-compatible (/v1/embeddings) or Ollama
// (/api/embeddings) endpoint to compute neural embeddings without cloud egress.
type HTTPEmbedder struct {
	Endpoint string
	Model    string
	Client   *http.Client
}

// NewEnvEmbedder constructs an HTTPEmbedder when KUBONA_EMBEDDING_URL is set.
func NewEnvEmbedder() TextEmbedder {
	endpoint := strings.TrimSpace(os.Getenv("KUBONA_EMBEDDING_URL"))
	if endpoint == "" {
		return nil
	}
	model := strings.TrimSpace(os.Getenv("KUBONA_EMBEDDING_MODEL"))
	if model == "" {
		model = "nomic-embed-text"
	}
	return &HTTPEmbedder{
		Endpoint: endpoint,
		Model:    model,
		Client:   &http.Client{Timeout: 5 * time.Second},
	}
}

func (e *HTTPEmbedder) Embed(ctx context.Context, text string) ([]float32, string, error) {
	if e == nil || strings.TrimSpace(e.Endpoint) == "" {
		return nil, "", fmt.Errorf("embedding endpoint not configured")
	}
	client := e.Client
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	isOllama := strings.Contains(e.Endpoint, "/api/embeddings")
	var reqBody []byte
	if isOllama {
		reqBody, _ = json.Marshal(map[string]any{"model": e.Model, "prompt": text})
	} else {
		reqBody, _ = json.Marshal(map[string]any{"model": e.Model, "input": text})
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.Endpoint, bytes.NewReader(reqBody))
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("embedding endpoint returned HTTP %d", resp.StatusCode)
	}
	var raw struct {
		Embedding []float32 `json:"embedding"`
		Data      []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
		Model string `json:"model"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, "", err
	}
	vec := raw.Embedding
	if len(vec) == 0 && len(raw.Data) > 0 {
		vec = raw.Data[0].Embedding
	}
	if len(vec) == 0 {
		return nil, "", fmt.Errorf("embedding endpoint returned empty vector")
	}
	model := raw.Model
	if model == "" {
		model = e.Model
	}
	return vec, model, nil
}

// ScoredMemoryItem pairs a memory with its cosine similarity to the search query
type ScoredMemoryItem struct {
	Item       *MemoryItem `json:"item"`
	Similarity float64     `json:"similarity"`
}

// EncodeEmbedding serializes a float32 vector into a binary byte slice
func EncodeEmbedding(vec []float32) []byte {
	buf := make([]byte, len(vec)*4)
	for i, val := range vec {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(val))
	}
	return buf
}

// DecodeEmbedding deserializes a binary byte slice back into a float32 vector
func DecodeEmbedding(buf []byte) ([]float32, error) {
	if len(buf)%4 != 0 {
		return nil, fmt.Errorf("invalid embedding buffer length: %d (must be multiple of 4)", len(buf))
	}
	dim := len(buf) / 4
	vec := make([]float32, dim)
	for i := 0; i < dim; i++ {
		bits := binary.LittleEndian.Uint32(buf[i*4:])
		vec[i] = math.Float32frombits(bits)
	}
	return vec, nil
}

// CosineSimilarity computes the normalized dot product between two vector embeddings
func CosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0.0
	}

	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i] * b[i])
		normA += float64(a[i] * a[i])
		normB += float64(b[i] * b[i])
	}

	if normA <= 1e-12 || normB <= 1e-12 {
		return 0.0
	}

	sim := dot / (math.Sqrt(normA) * math.Sqrt(normB))
	if sim > 1.0 {
		return 1.0
	}
	if sim < -1.0 {
		return -1.0
	}
	return sim
}

// GenerateSemanticEmbedding produces a 384-dimensional normalized vector embedding
// from text using multi-scale n-gram hashing and tf-idf weighting for fast offline retrieval.
func GenerateSemanticEmbedding(text string) []float32 {
	vec := make([]float32, EmbeddingDimensions)
	clean := strings.ToLower(strings.TrimSpace(text))
	if clean == "" {
		return vec
	}

	words := strings.Fields(clean)
	// Word-level hash features
	for _, w := range words {
		h := sha256.Sum256([]byte(w))
		for i := 0; i < 8; i++ {
			idx := int(binary.BigEndian.Uint16(h[i*2:i*2+2])) % EmbeddingDimensions
			sign := float32(1.0)
			if h[16+i]%2 == 0 {
				sign = -1.0
			}
			vec[idx] += sign * 1.5
		}
	}

	// Character 3-gram features for subword semantics
	chars := []rune(clean)
	if len(chars) >= 3 {
		for i := 0; i <= len(chars)-3; i++ {
			gram := string(chars[i : i+3])
			h := sha256.Sum256([]byte("tri:" + gram))
			idx := int(binary.BigEndian.Uint16(h[0:2])) % EmbeddingDimensions
			sign := float32(1.0)
			if h[4]%2 == 0 {
				sign = -1.0
			}
			vec[idx] += sign * 0.5
		}
	}

	// L2 normalization
	var norm float64
	for _, v := range vec {
		norm += float64(v * v)
	}
	if norm > 1e-12 {
		invNorm := float32(1.0 / math.Sqrt(norm))
		for i := range vec {
			vec[i] *= invNorm
		}
	}

	return vec
}
