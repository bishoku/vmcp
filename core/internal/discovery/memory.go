package discovery

import (
	"context"
	"math"
	"sort"
	"sync"
)

// MemoryVectorStore is a fast in-memory VectorStore for unit testing and local development.
type MemoryVectorStore struct {
	mu      sync.RWMutex
	tools   map[string]ToolRecord
	vectors map[string][]float32
}

// NewMemoryVectorStore creates an in-memory vector store.
func NewMemoryVectorStore() *MemoryVectorStore {
	return &MemoryVectorStore{
		tools:   make(map[string]ToolRecord),
		vectors: make(map[string][]float32),
	}
}

func (m *MemoryVectorStore) Name() string {
	return "memory"
}

func (m *MemoryVectorStore) Init(_ context.Context) error {
	return nil
}

func (m *MemoryVectorStore) Upsert(_ context.Context, tools []ToolRecord, vectors [][]float32) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, t := range tools {
		m.tools[t.ID] = t
		m.vectors[t.ID] = vectors[i]
	}
	return nil
}

func (m *MemoryVectorStore) Search(_ context.Context, query SearchQuery, queryVector []float32) ([]SearchResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	limit := query.Limit
	if limit <= 0 {
		limit = 10
	}

	var candidates []SearchResult
	for id, tool := range m.tools {
		if !tool.IsActive {
			continue
		}
		if query.Domain != "" && tool.Domain != query.Domain {
			continue
		}
		if query.ServerID != "" && tool.ServerID != query.ServerID {
			continue
		}
		// RBAC pre-filter
		if query.UserRole != "" {
			allowed := false
			for _, r := range tool.AllowedRoles {
				if r == query.UserRole || r == "*" {
					allowed = true
					break
				}
			}
			if !allowed {
				continue
			}
		}

		vec := m.vectors[id]
		score := CosineSimilarity(queryVector, vec)
		if query.MinScore > 0 && score < query.MinScore {
			continue
		}

		candidates = append(candidates, SearchResult{
			Tool:  tool,
			Score: score,
		})
	}

	// Sort descending by score
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Score > candidates[j].Score
	})

	if len(candidates) > limit {
		candidates = candidates[:limit]
	}
	return candidates, nil
}

func (m *MemoryVectorStore) Delete(_ context.Context, toolIDs []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, id := range toolIDs {
		delete(m.tools, id)
		delete(m.vectors, id)
	}
	return nil
}

func (m *MemoryVectorStore) Count(_ context.Context) (int64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return int64(len(m.tools)), nil
}

func (m *MemoryVectorStore) Close() error {
	return nil
}

// CosineSimilarity computes dot product of two normalized vectors.
func CosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float32
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA <= 0 || normB <= 0 {
		return 0
	}
	return dot / (float32(math.Sqrt(float64(normA))) * float32(math.Sqrt(float64(normB))))
}
