//go:build acceptance

package omni

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"testing"
	"unicode"
)

type qualityCase struct {
	Language string   `json:"language"`
	Query    string   `json:"query"`
	Expected []string `json:"expected"`
	Kind     string   `json:"kind"`
}
type qualityCorpus struct {
	Scope     string        `json:"scope"`
	Documents []Document    `json:"documents"`
	Queries   []qualityCase `json:"queries"`
	Negatives []qualityCase `json:"negative_queries"`
}

func loadQualityCorpus(t *testing.T) qualityCorpus {
	t.Helper()
	data, err := os.ReadFile("../../../benchmark/omni/quality-cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var c qualityCorpus
	if err := json.Unmarshal(data, &c); err != nil {
		t.Fatal(err)
	}
	for n := range c.Documents {
		d := &c.Documents[n]
		d.Ref = Ref("business", d.Name)
		d.Server = "business"
		d.ServerName = "Business operations"
		d.ServerDescription = "Business capabilities for customer, banking, employee and infrastructure operations."
	}
	domains := []string{"supplier compliance", "vehicle maintenance", "building ventilation", "advertising campaign", "manufacturing quality", "agricultural irrigation", "laboratory sample", "insurance underwriting", "legal contract", "school enrollment"}
	for n := len(c.Documents); n < 1000; n++ {
		name := fmt.Sprintf("dataset_%03d", n)
		c.Documents = append(c.Documents, Document{Ref: Ref("operations", name), Server: "operations", Name: name, Description: fmt.Sprintf("Retrieve %s historical statistics for region %d and reporting period %d.", domains[n%len(domains)], n/10%10, n/100)})
	}
	return c
}

type qualityCache struct {
	remote Embedder
	cache  map[string][]float32
}

func (c *qualityCache) DocumentFormat() string { return "tool-first-v1" }
func (c *qualityCache) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	missing := []string{}
	positions := []int{}
	for n, text := range texts {
		if v, ok := c.cache[text]; ok {
			out[n] = v
		} else {
			missing = append(missing, text)
			positions = append(positions, n)
		}
	}
	if len(missing) > 0 {
		v, err := c.remote.Embed(ctx, missing)
		if err != nil {
			return nil, err
		}
		for n, p := range positions {
			out[p] = v[n]
			c.cache[missing[n]] = v[n]
		}
	}
	return out, nil
}

type qualityRepresentation struct {
	*qualityCache
	format string
}

func (c *qualityRepresentation) DocumentFormat() string { return c.format }
func (c *qualityRepresentation) ProfileID() string      { return c.format }
func TestDiscoveryQualityComparison(t *testing.T) {
	endpoint := os.Getenv("VMCP_TEST_EMBEDDING_URL")
	if endpoint == "" {
		t.Skip("real model endpoint not supplied")
	}
	corpus := loadQualityCorpus(t)
	docs := corpus.Documents
	access := Access{Refs: map[string]string{}}
	for _, d := range docs {
		access.Refs[d.Ref] = d.Hash()
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _ = json.NewEncoder(w).Encode(Catalog{Documents: docs}) }))
	defer srv.Close()
	backend, _ := NewBackend(srv.URL, strings.Repeat("x", 32))
	remote, err := NewHTTPEmbedder(endpoint, os.Getenv("VMCP_TEST_EMBEDDING_MODEL"), "", false)
	if err != nil {
		t.Fatal(err)
	}
	defer remote.Client.CloseIdleConnections()
	cache := &qualityCache{remote: remote, cache: map[string][]float32{}}
	baseline := []entry{}
	for start := 0; start < len(docs); start += 32 {
		batch := docs[start:min(start+32, len(docs))]
		texts := []string{}
		for _, d := range batch {
			texts = append(texts, legacyText(d))
		}
		vectors, err := cache.Embed(context.Background(), texts)
		if err != nil {
			t.Fatal(err)
		}
		for n, d := range batch {
			baseline = append(baseline, entry{Document: d, Hash: d.Hash(), Vector: vectors[n]})
		}
	}
	defaultIndex := NewIndex(&qualityRepresentation{cache, "legacy-v1"})
	defaultIndex.Refresh(context.Background(), backend)
	if defaultIndex.Status().Error != "" {
		t.Fatal(defaultIndex.Status())
	}
	index := NewIndex(&qualityRepresentation{cache, "tool-first-v1"})
	index.Refresh(context.Background(), backend)
	if index.Status().Error != "" {
		t.Fatal(index.Status())
	}
	rows := []map[string]any{}
	summary := map[string]map[string]float64{}
	for _, variant := range []string{"baseline", "default-current", "structured-simple", "structured-weighted", "structured-rrf", "structured-simple-floor035"} {
		index.ranking = "weighted"
		index.MinSimilarity = 0
		if variant == "structured-simple" || variant == "structured-simple-floor035" {
			index.ranking = "simple"
		}
		if variant == "structured-simple-floor035" {
			index.MinSimilarity = 0.35
		}
		if variant == "structured-rrf" {
			index.ranking = "rrf"
		}
		totals := map[string]float64{}
		for _, q := range append(corpus.Queries, corpus.Negatives...) {
			var hits []Hit
			if variant == "default-current" {
				var err error
				hits, _, err = defaultIndex.Search(context.Background(), access, q.Query, "", "", 5)
				if err != nil {
					t.Fatal(err)
				}
			} else if variant == "baseline" {
				vectors, err := cache.Embed(context.Background(), []string{q.Query})
				if err != nil {
					t.Fatal(err)
				}
				hits = legacyRank(baseline, access, q.Query, vectors[0], 5)
			} else {
				var err error
				hits, _, err = index.Search(context.Background(), access, q.Query, "", "", 5)
				if err != nil {
					t.Fatal(err)
				}
			}
			rank := 0
			names := []string{}
			for n, h := range hits {
				names = append(names, h.Name)
				for _, want := range q.Expected {
					if want == h.Name && (rank == 0 || n+1 < rank) {
						rank = n + 1
					}
				}
			}
			group := q.Language
			if len(q.Expected) == 0 {
				group = "negative"
			}
			totals[group+"_queries"]++
			if rank == 1 {
				totals[group+"_hit1"]++
			}
			if rank > 0 {
				totals[group+"_hit5"]++
				totals[group+"_reciprocal_rank_sum"] += 1 / float64(rank)
			}
			if len(q.Expected) == 0 && len(hits) > 0 {
				totals["negative_false_positive_queries"]++
			}
			rows = append(rows, map[string]any{"variant": variant, "case": q, "rank_at_5": rank, "tools": names})
		}
		summary[variant] = totals
		t.Log(variant, totals)
	}
	// Promotion gate: new defaults must not regress either language or negatives.
	for _, key := range []string{"en_hit1", "en_hit5", "en_reciprocal_rank_sum", "tr_hit1", "tr_hit5", "tr_reciprocal_rank_sum"} {
		if summary["default-current"][key]+1e-9 < summary["baseline"][key] {
			t.Errorf("default regression: %s", key)
		}
	}
	if summary["default-current"]["negative_false_positive_queries"] > summary["baseline"]["negative_false_positive_queries"] {
		t.Error("default false positives regressed")
	}
	if path := os.Getenv("VMCP_TEST_QUALITY_REPORT"); path != "" {
		out, _ := json.MarshalIndent(map[string]any{"model": remote.Model, "catalog_size": len(docs), "scope": corpus.Scope, "summary": summary, "results": rows}, "", "  ")
		if err := os.WriteFile(path, out, 0600); err != nil {
			t.Fatal(err)
		}
	}
}
func legacyText(d Document) string {
	return d.ServerName + " " + d.ServerDescription + " " + d.Name + " " + d.Description
}
func legacyRank(entries []entry, access Access, query string, vector []float32, limit int) []Hit {
	words := strings.FieldsFunc(strings.ToLower(query), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	hits := []Hit{}
	for _, e := range entries {
		if access.Refs[e.Ref] != e.Hash {
			continue
		}
		text := " " + strings.Join(strings.FieldsFunc(strings.ToLower(legacyText(e.Document)), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }), " ") + " "
		lex := 0.0
		for _, word := range words {
			if strings.Contains(text, " "+word+" ") {
				lex++
			}
		}
		if len(words) > 0 {
			lex /= float64(len(words))
		}
		score := 0.2*lex + 0.8*positiveCosine(vector, e.Vector)
		if score > 0 {
			hits = append(hits, Hit{Document: e.Document, Score: score})
		}
	}
	sort.Slice(hits, func(a, b int) bool { return better(hits[a], hits[b]) })
	return hits[:min(limit, len(hits))]
}
