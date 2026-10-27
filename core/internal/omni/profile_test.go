package omni

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type dimensionEmbedder struct {
	dimension int
	profile   string
	calls     int
}

func (e *dimensionEmbedder) ProfileID() string { return e.profile }
func (e *dimensionEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	e.calls++
	v := make([][]float32, len(texts))
	for n := range v {
		v[n] = make([]float32, e.dimension)
		v[n][0] = 1
	}
	return v, nil
}
func TestProfileDimensionDriftAndCoverage(t *testing.T) {
	docs := []Document{}
	for n := 0; n < 33; n++ {
		docs = append(docs, Document{Ref: Ref("chat", string(rune('A'+n))), Name: "orders", Description: "Read customer orders"})
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _ = json.NewEncoder(w).Encode(Catalog{Documents: docs}) }))
	defer srv.Close()
	b, _ := NewBackend(srv.URL, strings.Repeat("x", 32))
	e := &dimensionEmbedder{dimension: 2, profile: "model-v1"}
	i := NewIndex(e)
	i.Refresh(context.Background(), b)
	if s := i.Status(); s.EmbeddedDocuments != 33 || s.Dimensions != 2 || s.Mode != "hybrid" {
		t.Fatal(s)
	}
	calls := e.calls
	i.Refresh(context.Background(), b)
	if e.calls != calls {
		t.Fatal("unchanged vectors recomputed")
	}
	access := Access{Refs: map[string]string{}}
	for _, d := range docs {
		access.Refs[d.Ref] = d.Hash()
	}
	e.dimension = 3
	_, mode, err := i.Search(context.Background(), access, "orders", "", "", 5)
	if err != nil || mode != "keyword-fallback" {
		t.Fatalf("dimension drift not rejected: %s %v", mode, err)
	}
	// A new document from a changed provider must not mix dimensions into the index.
	docs = append(docs, Document{Ref: Ref("chat", "new"), Name: "orders"})
	access.Refs[docs[33].Ref] = docs[33].Hash()
	i.Refresh(context.Background(), b)
	if s := i.Status(); s.Mode != "hybrid-degraded" || s.EmbeddedDocuments != 33 || s.Dimensions != 2 {
		t.Fatal(s)
	}
	calls = e.calls
	_, mode, err = i.Search(context.Background(), access, "orders", "", "", 5)
	if err != nil || mode != "hybrid-degraded" || e.calls != calls {
		t.Fatalf("partial index was not consistent lexical fallback: %s %v", mode, err)
	}
	e.profile = "model-v2"
	i.Refresh(context.Background(), b)
	if s := i.Status(); s.EmbeddedDocuments != 34 || s.Dimensions != 3 || s.EmbeddingProfile != "model-v2" {
		t.Fatal(s)
	}
	_, mode, err = i.Search(context.Background(), access, "orders", "", "", 5)
	if err != nil || mode != "hybrid" {
		t.Fatalf("new profile not searchable: %s %v", mode, err)
	}
}
func TestEmbeddingPrefixesDimensionsAndKeylessProvider(t *testing.T) {
	var inputs []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Error("keyless provider got authorization header")
		}
		var body map[string]json.RawMessage
		_ = json.NewDecoder(r.Body).Decode(&body)
		if _, ok := body["dimensions"]; ok {
			t.Error("expected dimensions must not change provider request contract")
		}
		_ = json.Unmarshal(body["input"], &inputs)
		_, _ = w.Write([]byte(`{"data":[{"index":0,"embedding":[1,0]}]}`))
	}))
	defer srv.Close()
	e, _ := NewHTTPEmbedder(srv.URL, "model", "", false)
	defer e.Client.CloseIdleConnections()
	e.Profile = EmbeddingProfile{QueryPrefix: "query: ", DocumentPrefix: "passage: ", Dimensions: 2, Revision: "v1"}
	p := e.ProfileID()
	if _, err := e.Embed(context.Background(), []string{"orders"}); err != nil || inputs[0] != "passage: orders" {
		t.Fatalf("document prefix: %v %v", inputs, err)
	}
	if _, err := e.EmbedQuery(context.Background(), []string{"orders"}); err != nil || inputs[0] != "query: orders" {
		t.Fatalf("query prefix: %v %v", inputs, err)
	}
	e.Profile.Dimensions = 3
	if e.ProfileID() == p {
		t.Fatal("profile did not change")
	}
	if _, err := e.EmbedQuery(context.Background(), []string{"orders"}); err == nil {
		t.Fatal("expected dimension ignored")
	}
	for _, v := range [][][]float32{{{0, 0}}, {{1}, {1, 0}}} {
		if _, err := validateVectors(v, len(v), 0); err == nil {
			t.Fatal("invalid vector accepted")
		}
	}
}

func TestMinimumSimilarityAndAuthorizedCoverage(t *testing.T) {
	docs := []Document{{Ref: Ref("chat", "orders"), Name: "orders", Description: "Read orders"}, {Ref: Ref("private", "orders"), Name: "orders", Description: "Read private orders"}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _ = json.NewEncoder(w).Encode(Catalog{Documents: docs}) }))
	defer srv.Close()
	b, _ := NewBackend(srv.URL, strings.Repeat("s", 32))
	e := &dimensionEmbedder{dimension: 2, profile: "v1"}
	i := NewIndex(e)
	i.Refresh(context.Background(), b)
	access := Access{Refs: map[string]string{docs[0].Ref: docs[0].Hash()}}
	// An inaccessible tool's missing vector cannot degrade authorized search.
	i.entries[1].Vector = nil
	hits, mode, err := i.Search(context.Background(), access, "orders", "", "", 5)
	if err != nil || mode != "hybrid" || len(hits) != 1 {
		t.Fatalf("private coverage influenced result: %s %v", mode, err)
	}
	// Lexical matches cannot bypass an explicitly configured semantic floor.
	i.entries[0].Vector = []float32{0, 1}
	i.MinSimilarity = 0.5
	hits, mode, err = i.Search(context.Background(), access, "orders", "", "", 5)
	if err != nil || mode != "hybrid" || len(hits) != 0 {
		t.Fatalf("similarity floor ignored: %v %s %v", hits, mode, err)
	}
}

func TestToolFirstTextAndProfileIdentity(t *testing.T) {
	d := Document{Name: "get_order_status", Description: "Read the fulfillment state of an existing order.", ServerName: "Customer platform", ServerDescription: strings.Repeat("Shared customer platform context. ", 2000)}
	structured := d.StructuredText()
	if !strings.HasPrefix(structured, "Tool: get_order_status\nPurpose: Read the fulfillment") || len([]rune(structured)) > 2500 {
		t.Fatal("capability purpose was hidden by shared context")
	}
	e, _ := NewHTTPEmbedder("http://localhost/v1/embeddings", "model", "", false)
	e.Profile.TextFormat = "legacy-v1"
	old := e.ProfileID()
	if documentText(d, e) != d.Text() {
		t.Fatal("default representation changed")
	}
	e.Profile.TextFormat = "tool-first-v1"
	if e.ProfileID() == old || documentText(d, e) != structured {
		t.Fatal("text format not part of embedding identity")
	}
	old = e.ProfileID()
	e.Profile.Revision = "changed-weights"
	if e.ProfileID() == old {
		t.Fatal("model revision not part of embedding identity")
	}
	if got := strings.Join(terms("getCPUUsage customerOrderID"), " "); got != "get cpu usage customer order id" {
		t.Fatal(got)
	}
}
