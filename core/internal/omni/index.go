package omni

import (
	"container/heap"
	"context"
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
	SearchText string
	Document
	Hash   string
	Vector []float32
}
type IndexStatus struct {
	Ready      bool      `json:"ready"`
	Mode       string    `json:"mode"`
	Documents  int       `json:"documents"`
	Revision   int64     `json:"revision"`
	UpdatedAt  time.Time `json:"updatedAt"`
	Error      string    `json:"error,omitempty"`
	Refreshing bool      `json:"refreshing"`
}
type Index struct {
	mu       sync.RWMutex
	entries  []entry
	status   IndexStatus
	Embedder Embedder
	refresh  chan struct{}
	runMu    sync.Mutex
}

func NewIndex(e Embedder) *Index     { return &Index{Embedder: e, refresh: make(chan struct{}, 1)} }
func (i *Index) Status() IndexStatus { i.mu.RLock(); defer i.mu.RUnlock(); return i.status }
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
		errText = "Published catalog unavailable"
		return
	}
	if len(catalog.Documents) > 100000 {
		errText = "Catalog exceeds 100000 documents"
		return
	}
	i.mu.RLock()
	old := make(map[string]entry, len(i.entries))
	for _, e := range i.entries {
		old[e.Ref] = e
	}
	i.mu.RUnlock()
	next := make([]entry, 0, len(catalog.Documents))
	pending := []int{}
	for _, d := range catalog.Documents {
		e := entry{Document: d, Hash: d.Hash(), SearchText: " " + strings.Join(terms(d.Text()), " ") + " "}
		if prev, ok := old[d.Ref]; ok && prev.Hash == e.Hash {
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
			texts = append(texts, next[j].Text())
		}
		vectors, err := i.Embedder.Embed(ctx, texts)
		if err != nil || len(vectors) != len(texts) {
			errText = "Embedding refresh failed; keyword search remains available"
			break
		}
		for n, j := range pending[start:end] {
			next[j].Vector = vectors[n]
		}
	}
	mode := "keyword"
	if i.Embedder != nil {
		mode = "hybrid"
		if errText != "" {
			mode = "hybrid-degraded"
		}
	}
	i.mu.Lock()
	i.entries = next
	i.status = IndexStatus{Ready: true, Mode: mode, Documents: len(next), Revision: catalog.Revision, UpdatedAt: time.Now(), Refreshing: true}
	i.mu.Unlock()
}

type Hit struct {
	Document
	Score float64 `json:"score"`
}

func terms(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
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
	entries, ready := i.entries, i.status.Ready
	i.mu.RUnlock()
	if !ready {
		return nil, "", &HTTPError{503}
	}
	if limit < 1 || limit > 50 {
		return nil, "", &HTTPError{400}
	}
	var vector []float32
	mode := "keyword"
	if i.Embedder != nil {
		v, err := i.Embedder.Embed(ctx, []string{query})
		if err == nil && len(v) == 1 {
			vector = v[0]
			mode = "hybrid"
		} else {
			mode = "keyword-fallback"
		}
	}
	words := terms(query)
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
		lex := 0.0
		for _, word := range words {
			if strings.Contains(e.SearchText, " "+word+" ") {
				lex++
			}
		}
		if len(words) > 0 {
			lex /= float64(len(words))
		}
		score := lex
		if len(vector) > 0 && len(e.Vector) == len(vector) {
			// Intent search is primarily semantic. Exact lexical tokens add a small
			// signal without substring matches or common words dominating meaning.
			score = 0.2*lex + 0.8*math.Max(0, cosine(vector, e.Vector))
		}
		if score <= 0 {
			continue
		}
		h := Hit{Document: e.Document, Score: score}
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
	sort.Slice(best, func(a, b int) bool { return better(best[a], best[b]) })
	return []Hit(best), mode, nil
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
