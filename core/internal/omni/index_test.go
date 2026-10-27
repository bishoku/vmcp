package omni

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type meaningEmbedder struct{}

func (meaningEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	v := make([][]float32, len(texts))
	for i, s := range texts {
		if strings.Contains(s, "orders") || strings.Contains(s, "sipariş") {
			v[i] = []float32{1, 0}
		} else {
			v[i] = []float32{0, 1}
		}
	}
	return v, nil
}
func TestHybridSearchFiltersBeforeRankingAndRejectsStaleMetadata(t *testing.T) {
	docs := []Document{{Ref: Ref("chat", "orders"), Server: "chat", Name: "orders", Description: "Open purchases"}, {Ref: Ref("hr", "salary"), Server: "hr", Name: "salary", Description: "orders orders sipariş"}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(Catalog{Revision: 1, Documents: docs})
	}))
	defer srv.Close()
	b, _ := NewBackend(srv.URL, strings.Repeat("x", 32))
	i := NewIndex(meaningEmbedder{})
	i.Refresh(context.Background(), b)
	a := Access{Refs: map[string]string{docs[0].Ref: docs[0].Hash()}}
	hits, mode, err := i.Search(context.Background(), a, "sipariş", "", "", 1)
	if err != nil || mode != "hybrid" || len(hits) != 1 || hits[0].Server != "chat" {
		t.Fatalf("authorized semantic search: %v %s %v", hits, mode, err)
	}
	a.Refs[docs[0].Ref] = "changed"
	hits, _, _ = i.Search(context.Background(), a, "sipariş", "", "", 1)
	if len(hits) != 0 {
		t.Fatal("stale metadata was exposed")
	}
	delete(a.Refs, docs[0].Ref)
	hits, _, _ = i.Search(context.Background(), a, "orders", "", "", 1)
	if len(hits) != 0 {
		t.Fatal("revoked tool was exposed")
	}
}
func TestEmbeddingResponseValidation(t *testing.T) {
	for _, body := range []string{`{"data":[{"index":0,"embedding":[1]},{"index":0,"embedding":[2]}]}`, `{"data":[{"index":0,"embedding":[]},{"index":1,"embedding":[]}]}`, `{"data":[{"index":0,"embedding":[1]},{"index":1,"embedding":[1,2]}]}`} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(body)) }))
		e, _ := NewHTTPEmbedder(srv.URL, "test", "", false)
		_, err := e.Embed(context.Background(), []string{"one", "two"})
		srv.Close()
		if err == nil {
			t.Fatal("accepted malformed embeddings")
		}
	}
}
func TestReferenceRoundTrip(t *testing.T) {
	s, n, e := ParseRef(Ref("chat", "客户/lookup"))
	if e != nil || s != "chat" || n != "客户/lookup" {
		t.Fatal(s, n, e)
	}
	if _, _, e := ParseRef("invalid"); e == nil {
		t.Fatal("invalid reference accepted")
	}
}

func BenchmarkAuthorizedSearch(b *testing.B) {
	for _, size := range []int{6000, 60000} {
		b.Run(fmt.Sprint(size), func(b *testing.B) {
			i := NewIndex(nil)
			i.status.Ready = true
			access := Access{Refs: map[string]string{}}
			for n := 0; n < size; n++ {
				d := Document{Ref: Ref("chat", fmt.Sprint(n)), Server: "chat", Name: fmt.Sprintf("orders_%d", n), Description: "Read customer orders and shipment status"}
				hash := d.Hash()
				i.entries = append(i.entries, entry{Document: d, Hash: hash, SearchText: strings.ToLower(d.Text())})
				access.Refs[d.Ref] = hash
			}
			b.ReportAllocs()
			b.ResetTimer()
			for n := 0; n < b.N; n++ {
				_, _, err := i.Search(context.Background(), access, "customer orders", "", "", 10)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func TestSearchTopKAndDistinctServers(t *testing.T) {
	docs := []Document{}
	for n := 0; n < 30; n++ {
		docs = append(docs, Document{Ref: Ref("chat", fmt.Sprint(n)), Server: "chat", Name: "orders", Description: "customer orders"})
	}
	docs = append(docs, Document{Ref: Ref("sales", "lookup"), Server: "sales", Name: "orders"})
	i := NewIndex(nil)
	i.status.Ready = true
	a := Access{Refs: map[string]string{}}
	for _, d := range docs {
		hash := d.Hash()
		i.entries = append(i.entries, entry{Document: d, Hash: hash, SearchText: strings.ToLower(d.Text())})
		a.Refs[d.Ref] = hash
	}
	hits, _, err := i.Search(context.Background(), a, "customer orders", "", "", 3)
	if err != nil || len(hits) != 3 {
		t.Fatal(hits, err)
	}
	for n := 1; n < len(hits); n++ {
		if better(hits[n], hits[n-1]) {
			t.Fatal("incorrect ranking")
		}
	}
	servers, _, err := i.SearchServers(context.Background(), a, "orders", "", "", 3)
	if err != nil || len(servers) != 2 || servers[0].Server == servers[1].Server {
		t.Fatal("servers not distinct", servers, err)
	}
}

func TestDiscoveryRechecksAuthorizationAfterRanking(t *testing.T) {
	d := Document{Ref: Ref("chat", "orders"), Server: "chat", Name: "orders", Description: "private matching metadata"}
	var calls int
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		a := Access{Refs: map[string]string{}}
		if calls < 3 {
			a.Refs[d.Ref] = d.Hash()
		}
		_ = json.NewEncoder(w).Encode(a)
	}))
	defer backend.Close()
	b, _ := NewBackend(backend.URL, strings.Repeat("x", 32))
	i := NewIndex(nil)
	i.status.Ready = true
	i.entries = []entry{{Document: d, Hash: d.Hash(), SearchText: strings.ToLower(d.Text())}}
	s := &Server{Backend: b, Index: i}
	handler := s.Handler()
	r := httptest.NewRequest("POST", "/mcp/v1/omni", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"discover_tools","arguments":{"query":"orders"}}}`))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "Bearer test")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 200 || calls != 3 || strings.Contains(w.Body.String(), d.Description) {
		t.Fatalf("revocation during search leaked metadata: %d %d %s", w.Code, calls, w.Body)
	}
}

func TestKeywordSearchDoesNotMatchInsideWords(t *testing.T) {
	d := Document{Ref: Ref("hr", "salary"), Server: "hr", Name: "salary", Description: "Monthly compensation"}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(Catalog{Documents: []Document{d}})
	}))
	defer srv.Close()
	backend, _ := NewBackend(srv.URL, strings.Repeat("x", 32))
	index := NewIndex(nil)
	index.Refresh(context.Background(), backend)
	access := Access{Refs: map[string]string{d.Ref: d.Hash()}}
	hits, _, err := index.Search(context.Background(), access, "al", "", "", 5)
	if err != nil || len(hits) != 0 {
		t.Fatalf("substring leaked into lexical matches: %v %v", hits, err)
	}
	hits, _, err = index.Search(context.Background(), access, "salary", "", "", 5)
	if err != nil || len(hits) != 1 {
		t.Fatalf("exact token lost: %v %v", hits, err)
	}
}
