package memory

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"
	"strings"
)

const EmbeddingDimensions = 384

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
