package omni

import (
	"container/heap"
	"context"
	"math"
	"sort"
	"strings"
	"unicode"
)

const defaultRanking = "simple"

// Weight specific capability fields above repeated service boilerplate. Scores
// are bounded; this is lexical retrieval, not a relevance probability.
func fieldLexical(e entry, words []string) float64 {
	if len(words) == 0 {
		return 0
	}
	name, purpose, context := e.NameText, e.PurposeText, e.ContextText
	if name == "" {
		prepared := prepareEntry(e.Document)
		name, purpose, context = prepared.NameText, prepared.PurposeText, prepared.ContextText
	}
	score := 0.0
	for _, word := range words {
		token := " " + word + " "
		n, p, c := strings.Contains(name, token), strings.Contains(purpose, token), strings.Contains(context, token)
		// A field match is useful on its own; matching specific fields adds a boost.
		if n {
			score += 1
		} else if p {
			score += 0.8
		} else if c {
			score += 0.2
		}
	}
	return score / float64(len(words))
}
func identifierWords(s string) string {
	r := []rune(s)
	var b strings.Builder
	for n, c := range r {
		if n > 0 && unicode.IsUpper(c) && (unicode.IsLower(r[n-1]) || (n+1 < len(r) && unicode.IsLower(r[n+1]) && unicode.IsUpper(r[n-1]))) {
			b.WriteByte(' ')
		}
		b.WriteRune(c)
	}
	return b.String()
}
func positiveCosine(a, b []float32) float64 { return math.Max(0, cosine(a, b)) }

func normalizedField(s string) string { return " " + strings.Join(terms(s), " ") + " " }
func prepareEntry(d Document) entry {
	return entry{Document: d, Hash: d.Hash(), SearchText: normalizedField(d.ServerName + " " + d.ServerDescription + " " + d.Name + " " + d.Description), NameText: normalizedField(d.Name), PurposeText: normalizedField(d.Description), ContextText: normalizedField(d.ServerName + " " + d.ServerDescription)}
}

// Bounded candidate fusion for comparison against the weighted baseline. Ranking
// sees only freshly authorized entries and never changes publication decisions.
func rankFusion(ctx context.Context, entries []entry, access Access, query, server, project string, words []string, vector []float32, limit int, groupServers bool) ([]Hit, error) {
	window := min(200, max(50, limit*4))
	lexical, dense := make(hitHeap, 0, window), make(hitHeap, 0, window)
	lexServers, denseServers := map[string]Hit{}, map[string]Hit{}
	offer := func(h Hit, target *hitHeap, groups map[string]Hit) {
		if h.Score <= 0 {
			return
		}
		if groupServers {
			prev, ok := groups[h.Server]
			if !ok || better(h, prev) {
				groups[h.Server] = h
			}
			return
		}
		offerBounded(target, h, window)
	}
	for n, e := range entries {
		if n%256 == 0 && ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if access.Refs[e.Ref] != e.Hash || (server != "" && e.Server != server) || (project != "" && e.Project != project) {
			continue
		}
		offer(Hit{Document: e.Document, Score: fieldLexical(e, words)}, &lexical, lexServers)
		if len(vector) > 0 {
			offer(Hit{Document: e.Document, Score: positiveCosine(vector, e.Vector)}, &dense, denseServers)
		}
	}
	for _, h := range lexServers {
		offerBounded(&lexical, h, window)
	}
	for _, h := range denseServers {
		offerBounded(&dense, h, window)
	}
	sort.Slice(lexical, func(a, b int) bool { return better(lexical[a], lexical[b]) })
	sort.Slice(dense, func(a, b int) bool { return better(dense[a], dense[b]) })
	fused := map[string]Hit{}
	for _, list := range []hitHeap{lexical, dense} {
		for rank, h := range list {
			key := h.Ref
			if groupServers {
				key = h.Server
			}
			prev, ok := fused[key]
			if !ok {
				prev = h
				prev.Score = 0
			}
			prev.Score += 1 / float64(60+rank+1)
			fused[key] = prev
		}
	}
	best := make(hitHeap, 0, limit)
	for _, h := range fused {
		offerBounded(&best, h, limit)
	}
	sort.Slice(best, func(a, b int) bool { return better(best[a], best[b]) })
	return []Hit(best), nil
}
func offerBounded(target *hitHeap, h Hit, limit int) {
	if len(*target) < limit {
		heap.Push(target, h)
	} else if better(h, (*target)[0]) {
		(*target)[0] = h
		heap.Fix(target, 0)
	}
}

func simpleLexical(e entry, words []string) float64 {
	if len(words) == 0 {
		return 0
	}
	score := 0.0
	for _, word := range words {
		if strings.Contains(e.SearchText, " "+word+" ") {
			score++
		}
	}
	return score / float64(len(words))
}
