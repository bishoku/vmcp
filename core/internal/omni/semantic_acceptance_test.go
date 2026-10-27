//go:build acceptance

package omni

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// Opt-in real-model evaluation. Synthetic corpus, not production-domain relevance.
func TestRealModelSemanticAcceptance(t *testing.T) {
	endpoint := os.Getenv("VMCP_TEST_EMBEDDING_URL")
	if endpoint == "" {
		t.Skip("real model endpoint not supplied")
	}
	model := os.Getenv("VMCP_TEST_EMBEDDING_MODEL")
	f, err := os.Open("../../../benchmark/omni/intents.tsv")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	reader := csv.NewReader(f)
	reader.Comma = '\t'
	examples, err := reader.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	docs := []Document{}
	access := Access{Refs: map[string]string{}}
	for _, e := range examples {
		docs = append(docs, Document{Ref: Ref("business", e[0]), Server: "business", Name: e[0], Description: e[1]})
	}
	domains := []string{"supplier compliance", "vehicle maintenance", "building ventilation", "advertising campaign", "manufacturing quality", "agricultural irrigation", "laboratory sample", "insurance underwriting", "legal contract", "school enrollment"}
	for n := 0; n < 1000-len(examples); n++ {
		name := fmt.Sprintf("dataset_%03d", n)
		docs = append(docs, Document{Ref: Ref("operations", name), Server: "operations", Name: name, Description: fmt.Sprintf("Retrieve %s historical statistics for region %d and reporting period %d.", domains[n%len(domains)], n/10%10, n/100)})
	}
	for _, d := range docs {
		access.Refs[d.Ref] = d.Hash()
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(Catalog{Revision: 1, Documents: docs})
	}))
	defer srv.Close()
	backend, _ := NewBackend(srv.URL, strings.Repeat("x", 32))
	embedder, err := NewHTTPEmbedder(endpoint, model, "", false)
	if err != nil {
		t.Fatal(err)
	}
	index := NewIndex(embedder)
	index.Refresh(context.Background(), backend)
	if s := index.Status(); s.Mode != "hybrid" || s.Error != "" || s.Documents != 1000 {
		t.Fatal(s)
	}
	rows := []map[string]any{}
	passed := map[string]int{"en": 0, "tr": 0}
	for _, e := range examples {
		for lang, query := range map[string]string{"en": e[2], "tr": e[3]} {
			hits, mode, err := index.Search(context.Background(), access, query, "", "", 5)
			rank := 0
			for n, h := range hits {
				if h.Name == e[0] {
					rank = n + 1
				}
			}
			if rank > 0 {
				passed[lang]++
			}
			rows = append(rows, map[string]any{"language": lang, "query": query, "expected": e[0], "rank_at_5": rank, "mode": mode})
			if err != nil || mode != "hybrid" {
				t.Error("semantic search failed", err, mode)
			}
		}
	}
	report := map[string]any{"model": model, "catalog_size": len(docs), "queries": len(rows), "hits_at_5": passed, "results": rows, "scope": "synthetic bilingual smoke corpus; no production relevance claim"}
	if path := os.Getenv("VMCP_TEST_SEMANTIC_REPORT"); path != "" {
		b, _ := json.MarshalIndent(report, "", "  ")
		if err := os.WriteFile(path, b, 0600); err != nil {
			t.Error(err)
		}
	}
	t.Logf("Real model recall@5: EN %d/30 TR %d/30", passed["en"], passed["tr"])
	if passed["en"] < 27 || passed["tr"] < 27 {
		t.Error("recall@5 below 90% for a language")
	}
}
