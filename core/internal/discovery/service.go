package discovery

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// ToolDecision documents why a specific tool was selected by the discovery engine.
type ToolDecision struct {
	ToolID   string  `json:"tool_id"`
	Name     string  `json:"name"`
	Domain   string  `json:"domain"`
	Score    float32 `json:"score"`
	Rank     int     `json:"rank"`
	Rationale string `json:"rationale"`
}

// Service coordinates embeddings, vector storage, and semantic caching.
type Service struct {
	store    VectorStore
	embedder Embedder
	cache    SemanticCache
	logger   *slog.Logger
	cacheTTL time.Duration
}

// ServiceConfig defines parameters for DiscoveryService.
type ServiceConfig struct {
	Store    VectorStore
	Embedder Embedder
	Cache    SemanticCache
	Logger   *slog.Logger
	CacheTTL time.Duration
}

// NewService instantiates a DiscoveryService.
func NewService(cfg ServiceConfig) *Service {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.CacheTTL <= 0 {
		cfg.CacheTTL = 15 * time.Minute
	}
	if cfg.Cache == nil {
		cfg.Cache = NewMemorySemanticCache()
	}
	return &Service{
		store:    cfg.Store,
		embedder: cfg.Embedder,
		cache:    cfg.Cache,
		logger:   cfg.Logger,
		cacheTTL: cfg.CacheTTL,
	}
}

// StoreName returns the active vector store engine name.
func (s *Service) StoreName() string {
	if s.store != nil {
		return s.store.Name()
	}
	return "none"
}

// IndexTools generates embeddings and persists tool records into the active vector store.
func (s *Service) IndexTools(ctx context.Context, tools []ToolRecord) error {
	if len(tools) == 0 {
		return nil
	}

	texts := make([]string, len(tools))
	for i, t := range tools {
		texts[i] = BuildEmbeddingText(t)
	}

	vectors, err := s.embedder.Embed(ctx, texts)
	if err != nil {
		return fmt.Errorf("generate embeddings for indexing failed: %w", err)
	}

	if err := s.store.Upsert(ctx, tools, vectors); err != nil {
		return fmt.Errorf("vector store upsert failed: %w", err)
	}

	s.logger.Info("indexed tools into vector store",
		"store", s.store.Name(),
		"count", len(tools))
	return nil
}

// SearchTools discovers relevant tools using L1 semantic cache and vector store,
// logging rich decision rationale for each returned tool.
func (s *Service) SearchTools(ctx context.Context, query SearchQuery) ([]SearchResult, error) {
	start := time.Now()
	if query.Limit <= 0 {
		query.Limit = 10
	}

	s.logger.Debug("evaluating tool discovery query",
		"query", query.Query,
		"user_role", query.UserRole,
		"domain_filter", query.Domain,
		"limit", query.Limit,
		"store_engine", s.StoreName())

	// 1. Check L1 Semantic Cache (<0.4ms)
	if results, hit := s.cache.Get(ctx, query); hit {
		selectedNames := make([]string, len(results))
		for i, r := range results {
			selectedNames[i] = r.Tool.Name
		}
		s.logger.Info("tool discovery decision: cache hit",
			"query", query.Query,
			"user_role", query.UserRole,
			"selected_count", len(results),
			"selected_tools", selectedNames,
			"decision_source", "L1_semantic_cache",
			"latency_ms", time.Since(start).Milliseconds())
		return results, nil
	}

	// 2. Generate embedding for user query
	vectors, err := s.embedder.Embed(ctx, []string{query.Query})
	if err != nil {
		return nil, fmt.Errorf("embed query failed: %w", err)
	}
	if len(vectors) == 0 || len(vectors[0]) == 0 {
		return nil, fmt.Errorf("embed query returned empty vector")
	}

	// 3. Search Vector Store (Qdrant or pgvector) with RBAC pre-filter
	results, err := s.store.Search(ctx, query, vectors[0])
	if err != nil {
		return nil, fmt.Errorf("vector search failed: %w", err)
	}

	// 4. Construct Decision Reasoning & Log Analysis
	decisions := make([]ToolDecision, len(results))
	selectedNames := make([]string, len(results))
	for i, r := range results {
		selectedNames[i] = r.Tool.Name
		rationale := fmt.Sprintf("High semantic similarity (score=%.4f), matched active status, domain filter '%s' satisfied, RBAC role '%s' permitted",
			r.Score, r.Tool.Domain, query.UserRole)
		decisions[i] = ToolDecision{
			ToolID:    r.Tool.ID,
			Name:      r.Tool.Name,
			Domain:    r.Tool.Domain,
			Score:     r.Score,
			Rank:      i + 1,
			Rationale: rationale,
		}
	}

	s.logger.Info("tool discovery decision concluded",
		"query", query.Query,
		"user_role", query.UserRole,
		"domain_filter", query.Domain,
		"store_engine", s.StoreName(),
		"evaluated_candidates_count", len(results),
		"selected_count", len(results),
		"selected_tools", selectedNames,
		"decisions", decisions,
		"decision_source", "vector_hnsw_search",
		"duration_ms", float64(time.Since(start).Nanoseconds())/1e6)

	// 5. Save to L1 Semantic Cache
	s.cache.Set(ctx, query, results, s.cacheTTL)

	return results, nil
}

// DeleteTools removes tools from vector store and invalidates cache entries.
func (s *Service) DeleteTools(ctx context.Context, toolIDs []string) error {
	if len(toolIDs) == 0 {
		return nil
	}
	if err := s.store.Delete(ctx, toolIDs); err != nil {
		return err
	}
	for _, id := range toolIDs {
		s.cache.Invalidate(ctx, id)
	}
	return nil
}

// Count returns the number of tools indexed in the vector store.
func (s *Service) Count(ctx context.Context) (int64, error) {
	return s.store.Count(ctx)
}
