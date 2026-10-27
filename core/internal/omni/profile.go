package omni

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
)

// Change when the document representation changes so old vectors are never reused.
const textVersion = "embedding-contract-v2"

type EmbeddingProfile struct {
	TextFormat     string
	Revision       string
	QueryPrefix    string
	DocumentPrefix string
	Dimensions     int
}

func (e *HTTPEmbedder) ProfileID() string {
	b, _ := json.Marshal([]any{e.Endpoint, e.Model, e.Profile, textVersion})
	return fmt.Sprintf("%x", sha256.Sum256(b))
}
func (e *HTTPEmbedder) ExpectedDimensions() int { return e.Profile.Dimensions }
func (e *HTTPEmbedder) EmbedQuery(ctx context.Context, texts []string) ([][]float32, error) {
	return e.embed(ctx, texts, e.Profile.QueryPrefix)
}
func embeddingProfile(e Embedder) string {
	if p, ok := e.(interface{ ProfileID() string }); ok {
		return p.ProfileID()
	}
	return textVersion
}
func documentText(d Document, e Embedder) string {
	if f, ok := e.(interface{ DocumentFormat() string }); ok && f.DocumentFormat() == "tool-first-v1" {
		return d.StructuredText()
	}
	return d.Text()
}
func (e *HTTPEmbedder) DocumentFormat() string { return e.Profile.TextFormat }
func expectedDimensions(e Embedder) int {
	if p, ok := e.(interface{ ExpectedDimensions() int }); ok {
		return p.ExpectedDimensions()
	}
	return 0
}
func validateVectors(v [][]float32, count, dimension int) (int, error) {
	if len(v) != count {
		return dimension, &embeddingError{reason: "embedding_count_mismatch", attrs: []slog.Attr{slog.Int("expected_count", count), slog.Int("actual_count", len(v))}}
	}
	for _, vector := range v {
		if len(vector) < 1 || len(vector) > 4096 {
			return dimension, &embeddingError{reason: "invalid_embedding_dimensions", attrs: []slog.Attr{slog.Int("actual_dimensions", len(vector))}}
		}
		if dimension == 0 {
			dimension = len(vector)
		}
		if len(vector) != dimension {
			return dimension, &embeddingError{reason: "inconsistent_embedding_dimensions", attrs: []slog.Attr{slog.Int("expected_dimensions", dimension), slog.Int("actual_dimensions", len(vector))}}
		}
		norm := 0.0
		for _, value := range vector {
			if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
				return dimension, &embeddingError{reason: "nonfinite_embedding"}
			}
			norm += float64(value) * float64(value)
		}
		if norm == 0 {
			return dimension, &embeddingError{reason: "zero_embedding"}
		}
	}
	return dimension, nil
}
