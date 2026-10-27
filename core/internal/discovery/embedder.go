package discovery

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"
)

// TEIEmbedder calls HuggingFace Text Embeddings Inference (TEI) HTTP microservice.
type TEIEmbedder struct {
	endpoint   string
	dimension  int
	httpClient *http.Client
}

// NewTEIEmbedder creates a HuggingFace TEI client.
func NewTEIEmbedder(endpoint string, dimension int) *TEIEmbedder {
	if dimension <= 0 {
		dimension = 384 // Default for BAAI/bge-small-en-v1.5
	}
	tr := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 50,
		IdleConnTimeout:     90 * time.Second,
	}
	return &TEIEmbedder{
		endpoint:   strings.TrimRight(endpoint, "/"),
		dimension:  dimension,
		httpClient: &http.Client{Transport: tr, Timeout: 10 * time.Second},
	}
}

func (e *TEIEmbedder) Dimension() int {
	return e.dimension
}

// Embed calls POST /embed with JSON payload {"inputs": ["text1", "text2"]}.
func (e *TEIEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	reqBody, err := json.Marshal(map[string]any{
		"inputs":    texts,
		"normalize": true,
		"truncate":  true,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal embedding request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.endpoint+"/embed", bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("create tei request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tei embedding call failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tei embedding returned status %d", resp.StatusCode)
	}

	var rawVectors [][]float32
	if err := json.NewDecoder(resp.Body).Decode(&rawVectors); err != nil {
		return nil, fmt.Errorf("decode tei response: %w", err)
	}
	return rawVectors, nil
}

// DeterministicLocalEmbedder provides high-speed, zero-dependency 384-dimensional
// normalized vector embeddings. It uses token-level hashing and term weighting,
// ideal for local tests, development environments, and fallback modes.
type DeterministicLocalEmbedder struct {
	dimension int
}

// NewDeterministicLocalEmbedder creates a local zero-dependency embedder.
func NewDeterministicLocalEmbedder(dim int) *DeterministicLocalEmbedder {
	if dim <= 0 {
		dim = 384
	}
	return &DeterministicLocalEmbedder{dimension: dim}
}

func (d *DeterministicLocalEmbedder) Dimension() int {
	return d.dimension
}

func (d *DeterministicLocalEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	results := make([][]float32, len(texts))
	for i, text := range texts {
		results[i] = d.embedSingle(text)
	}
	return results, nil
}

func (d *DeterministicLocalEmbedder) embedSingle(text string) []float32 {
	vec := make([]float32, d.dimension)
	tokens := strings.Fields(strings.ToLower(text))
	if len(tokens) == 0 {
		return vec
	}

	for _, token := range tokens {
		h := sha256.Sum256([]byte(token))
		// Use hash bits to distribute energy across dimensions
		for j := 0; j < 4; j++ {
			idx := int(binary.BigEndian.Uint32(h[j*4:(j+1)*4])) % d.dimension
			val := float32(int16(binary.BigEndian.Uint16(h[16+j*2:18+j*2]))) / 32768.0
			vec[idx] += val
		}
	}

	// L2 Normalize
	var norm float64
	for _, v := range vec {
		norm += float64(v * v)
	}
	norm = math.Sqrt(norm)
	if norm > 1e-9 {
		inv := float32(1.0 / norm)
		for j := range vec {
			vec[j] *= inv
		}
	}
	return vec
}
