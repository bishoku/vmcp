package omni

import (
	"container/heap"
	"context"
	"errors"
	"math"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
)

type Embedder interface {
	Embed(context.Context, []string) ([][]float32, error)
}
type entry struct {
	SearchText                         string
	NameText, PurposeText, ContextText string
	Document
	Hash   string
	Vector []float32
}
type IndexStatus struct {
	Ready                 bool      `json:"ready"`
	Mode                  string    `json:"mode"`
	Documents             int       `json:"documents"`
	Revision              int64     `json:"revision"`
	UpdatedAt             time.Time `json:"updatedAt"`
	Error                 string    `json:"error,omitempty"`
	Refreshing            bool      `json:"refreshing"`
	EmbeddedDocuments     int       `json:"embeddedDocuments"`
	Dimensions            int       `json:"dimensions"`
	EmbeddingProfile      string    `json:"embeddingProfile,omitempty"`
	RerankEnabled         bool      `json:"rerankEnabled"`
	RerankCandidates      int       `json:"rerankCandidates"`
	RerankMinScore        float64   `json:"rerankMinScore"`
	RerankNoMatchScore    float64   `json:"rerankNoMatchScore"`
	RerankConfidenceScore float64   `json:"rerankConfidenceScore"`
	RerankFailurePolicy   string    `json:"rerankFailurePolicy"`
}
type Index struct {
	mu                    sync.RWMutex
	entries               []entry
	status                IndexStatus
	Embedder              Embedder
	refresh               chan struct{}
	runMu                 sync.Mutex
	catalogLog            failureLog
	refreshLog            failureLog
	searchLog             failureLog
	ranking               string
	MinSimilarity         float64
	Reranker              Reranker
	RerankCandidates      int
	RerankMinScore        float64
	RerankNoMatchScore    float64
	RerankConfidenceScore float64
	RerankFailurePolicy   string
	rerankLog             failureLog
}

func NewIndex(e Embedder) *Index {
	return &Index{Embedder: e, refresh: make(chan struct{}, 1), RerankCandidates: 20, RerankFailurePolicy: "fallback"}
}
func (i *Index) Status() IndexStatus {
	i.mu.RLock()
	defer i.mu.RUnlock()
	status := i.status
	status.RerankEnabled = i.Reranker != nil
	status.RerankCandidates = i.RerankCandidates
	status.RerankMinScore = i.RerankMinScore
	status.RerankNoMatchScore = i.RerankNoMatchScore
	status.RerankConfidenceScore = i.RerankConfidenceScore
	status.RerankFailurePolicy = i.RerankFailurePolicy
	return status
}
func (i *Index) Reindex() {
	select {
	case i.refresh <- struct{}{}:
	default:
	}
}
func (i *Index) Run(ctx context.Context, b *Backend, interval time.Duration) {
	timer := time.NewTicker(interval)
	defer timer.Stop()
	for {
		i.Refresh(ctx, b)
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		case <-i.refresh:
		}
	}
}
func (i *Index) Refresh(ctx context.Context, b *Backend) {
	i.runMu.Lock()
	defer i.runMu.Unlock()
	i.mu.Lock()
	i.status.Refreshing = true
	i.mu.Unlock()
	errText := ""
	defer func() { i.mu.Lock(); i.status.Refreshing = false; i.status.Error = errText; i.mu.Unlock() }()
	var catalog Catalog
	if err := b.Request(ctx, "GET", "/internal/omni/catalog", "", "", "", nil, &catalog); err != nil {
		i.catalogLog.report("catalog_refresh", err)
		errText = "Published catalog unavailable"
		return
	}
	i.catalogLog.report("catalog_refresh", nil)
	if len(catalog.Documents) > 100000 {
		i.refreshLog.report("index_refresh", &embeddingError{reason: "catalog_limit_exceeded"}, "documents", len(catalog.Documents))
		errText = "Catalog exceeds 100000 documents"
		return
	}
	profile := embeddingProfile(i.Embedder)
	i.mu.RLock()
	oldProfile, dimension := i.status.EmbeddingProfile, i.status.Dimensions
	if oldProfile != profile {
		dimension = expectedDimensions(i.Embedder)
	}
	old := make(map[string]entry, len(i.entries))
	for _, e := range i.entries {
		old[e.Ref] = e
	}
	i.mu.RUnlock()
	next := make([]entry, 0, len(catalog.Documents))
	pending := []int{}
	for _, d := range catalog.Documents {
		e := prepareEntry(d)
		if prev, ok := old[d.Ref]; ok && prev.Hash == e.Hash && oldProfile == profile {
			e.Vector = prev.Vector
		}
		next = append(next, e)
		if i.Embedder != nil && len(e.Vector) == 0 {
			pending = append(pending, len(next)-1)
		}
	}
	for start := 0; start < len(pending); start += 32 {
		end := min(start+32, len(pending))
		texts := []string{}
		for _, j := range pending[start:end] {
			texts = append(texts, documentText(next[j].Document, i.Embedder))
		}
		started := time.Now()
		vectors, err := i.Embedder.Embed(ctx, texts)
		if err == nil {
			validatedDimension, validationErr := validateVectors(vectors, len(texts), dimension)
			err = validationErr
			if err == nil {
				dimension = validatedDimension
			}
		}
		if err != nil {
			i.refreshLog.report("index_refresh", err, "batch_offset", start, "batch_size", len(texts), "pending_documents", len(pending), "documents", len(next), "elapsed_ms", time.Since(started).Milliseconds(), "fallback", "keyword")
			errText = "Embedding refresh failed; keyword search remains available"
			break
		}
		for n, j := range pending[start:end] {
			next[j].Vector = vectors[n]
		}
	}
	if errText == "" {
		i.refreshLog.report("index_refresh", nil, "documents", len(next))
	}
	mode := "keyword"
	if i.Embedder != nil {
		mode = "hybrid"
		if errText != "" {
			mode = "hybrid-degraded"
		}
	}
	embedded := 0
	for _, e := range next {
		if len(e.Vector) > 0 {
			embedded++
		}
	}
	i.mu.Lock()
	i.entries = next
	i.status = IndexStatus{Ready: true, Mode: mode, Documents: len(next), Revision: catalog.Revision, UpdatedAt: time.Now(), Refreshing: true, EmbeddedDocuments: embedded, Dimensions: dimension, EmbeddingProfile: profile}
	i.mu.Unlock()
}

type Hit struct {
	Document
	Score float64 `json:"score"`
}

func terms(s string) []string {
	return strings.FieldsFunc(strings.ToLower(identifierWords(s)), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
}
func cosine(a, b []float32) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, aa, bb float64
	for n, v := range a {
		x, y := float64(v), float64(b[n])
		dot += x * y
		aa += x * x
		bb += y * y
	}
	if aa == 0 || bb == 0 {
		return 0
	}
	return dot / math.Sqrt(aa*bb)
}

// Search filters by fresh authorization before scoring and limiting. It never caches credentials.
func (i *Index) Search(ctx context.Context, access Access, query, server, project string, limit int) ([]Hit, string, error) {
	return i.search(ctx, access, query, server, project, limit, false)
}
func (i *Index) SearchServers(ctx context.Context, access Access, query, server, project string, limit int) ([]Hit, string, error) {
	return i.search(ctx, access, query, server, project, limit, true)
}

// Retain only the best K matches; memory per query does not grow with catalog size.
func (i *Index) search(ctx context.Context, access Access, query, server, project string, limit int, groupServers bool) ([]Hit, string, error) {
	i.mu.RLock()
	entries, ready, dimension, profile := i.entries, i.status.Ready, i.status.Dimensions, i.status.EmbeddingProfile
	i.mu.RUnlock()
	if !ready {
		return nil, "", &HTTPError{503}
	}
	if limit < 1 || limit > 50 {
		return nil, "", &HTTPError{400}
	}
	resultLimit, resultServers := limit, groupServers
	if i.Reranker != nil {
		candidates := i.RerankCandidates
		if candidates <= 0 {
			candidates = 20
		}
		limit = min(100, max(limit, candidates))
		groupServers = false // Rerank tool candidates before collapsing server results.
	}
	var vector []float32
	mode := "keyword"
	eligible, embedded := 0, 0
	if i.Embedder != nil {
		for n, e := range entries {
			if n%256 == 0 && ctx.Err() != nil {
				return nil, "", ctx.Err()
			}
			if access.Refs[e.Ref] != e.Hash || (server != "" && e.Server != server) || (project != "" && e.Project != project) {
				continue
			}
			eligible++
			if len(e.Vector) > 0 && (dimension == 0 || len(e.Vector) == dimension) {
				embedded++
			}
		}
	}
	if i.Embedder != nil {
		mode = "hybrid-degraded"
		if eligible > 0 && embedded == eligible && profile == embeddingProfile(i.Embedder) {
			started := time.Now()
			embed := i.Embedder.Embed
			if q, ok := i.Embedder.(interface {
				EmbedQuery(context.Context, []string) ([][]float32, error)
			}); ok {
				embed = q.EmbedQuery
			}
			v, err := embed(ctx, []string{query})
			if err == nil {
				_, err = validateVectors(v, 1, dimension)
			}
			if err == nil {
				i.searchLog.report("search_embedding", nil)
				vector = v[0]
				mode = "hybrid"
			} else {
				if !errors.Is(ctx.Err(), context.Canceled) {
					i.searchLog.report("search_embedding", err, "elapsed_ms", time.Since(started).Milliseconds(), "fallback", "keyword")
				}
				mode = "keyword-fallback"
			}
		}
	}
	words := terms(query)
	ranking := i.ranking
	if ranking == "" {
		ranking = defaultRanking
	}
	if ranking == "rrf" {
		hits, err := rankFusion(ctx, entries, access, query, server, project, words, vector, limit, groupServers)
		if err != nil {
			return nil, mode, err
		}
		return i.refine(ctx, query, hits, resultLimit, resultServers, mode)
	}
	best := make(hitHeap, 0, limit)
	offer := func(h Hit) {
		if len(best) < limit {
			heap.Push(&best, h)
		} else if better(h, best[0]) {
			best[0] = h
			heap.Fix(&best, 0)
		}
	}
	var servers map[string]Hit
	var serverCandidates map[string]hitHeap
	if i.Reranker != nil && resultServers {
		serverCandidates = map[string]hitHeap{}
	}
	if groupServers {
		servers = map[string]Hit{}
	}
	for n, e := range entries {
		if n%256 == 0 && ctx.Err() != nil {
			return nil, "", ctx.Err()
		}
		if access.Refs[e.Ref] != e.Hash || (server != "" && e.Server != server) || (project != "" && e.Project != project) {
			continue
		}
		lex := simpleLexical(e, words)
		if ranking == "weighted" {
			lex = fieldLexical(e, words)
		}
		if len(vector) > 0 && positiveCosine(vector, e.Vector) < i.MinSimilarity {
			continue
		}
		score := lex
		if len(vector) > 0 && len(e.Vector) == len(vector) {
			score = 0.2*lex + 0.8*positiveCosine(vector, e.Vector)
		}
		if score <= 0 {
			continue
		}
		h := Hit{Document: e.Document, Score: score}
		if i.Reranker != nil && resultServers {
			candidates := serverCandidates[e.Server]
			offerBounded(&candidates, h, 3)
			serverCandidates[e.Server] = candidates
			continue
		}
		if groupServers {
			prev, exists := servers[e.Server]
			if !exists || better(h, prev) {
				servers[e.Server] = h
			}
		} else {
			offer(h)
		}
	}
	for _, h := range servers {
		offer(h)
	}
	if i.Reranker != nil && resultServers {
		best = hitHeap(balancedServerCandidates(serverCandidates, limit))
	}
	sort.Slice(best, func(a, b int) bool { return better(best[a], best[b]) })
	return i.refine(ctx, query, []Hit(best), resultLimit, resultServers, mode)
}
func better(a, b Hit) bool {
	if a.Score == b.Score {
		return a.Ref < b.Ref
	}
	return a.Score > b.Score
}

type hitHeap []Hit

func (h hitHeap) Len() int           { return len(h) }
func (h hitHeap) Less(i, j int) bool { return better(h[j], h[i]) }
func (h hitHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *hitHeap) Push(v any)        { *h = append(*h, v.(Hit)) }
func (h *hitHeap) Pop() any          { last := (*h)[len(*h)-1]; *h = (*h)[:len(*h)-1]; return last }
