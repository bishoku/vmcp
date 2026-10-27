package omni

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type rerankFunc func(context.Context, string, []Document) ([]float64, error)

func (f rerankFunc) Rerank(c context.Context, q string, d []Document) ([]float64, error) {
	return f(c, q, d)
}
func TestHTTPRerankContractAndValidation(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		valid      bool
	}{
		{"out_of_order", `{"results":[{"index":1,"relevance_score":0.9},{"index":0,"relevance_score":0.1}]}`, true},
		{"duplicate", `{"results":[{"index":0,"relevance_score":0.9},{"index":0,"relevance_score":0.1}]}`, false},
		{"missing", `{"results":[]}`, false},
		{"out_of_range", `{"results":[{"index":0,"relevance_score":1.1},{"index":1,"relevance_score":0.1}]}`, false},
		{"missing_score", `{"results":[{"index":0},{"index":1,"relevance_score":0.1}]}`, false},
		{"missing_index", `{"results":[{"relevance_score":0.9},{"index":1,"relevance_score":0.1}]}`, false},
		{"not_json", "private provider response", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "" {
					t.Error("keyless provider got auth header")
				}
				var p struct {
					Model, Query string
					Documents    []string
					TopN         int `json:"top_n"`
				}
				_ = json.NewDecoder(r.Body).Decode(&p)
				if p.Model != "model" || p.Query != "intent" || p.TopN != 2 || len(p.Documents) != 2 || !strings.HasPrefix(p.Documents[0], "Tool: first") {
					t.Errorf("request contract: %+v", p)
				}
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			e, err := NewHTTPReranker(srv.URL, "model", "", false, time.Second)
			if err != nil {
				t.Fatal(err)
			}
			defer e.Client.CloseIdleConnections()
			scores, err := e.Rerank(context.Background(), "intent", []Document{{Name: "first"}, {Name: "second"}})
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v err=%v", tc.valid, err)
			}
			if tc.valid && (scores[0] != 0.1 || scores[1] != 0.9) {
				t.Fatal(scores)
			}
		})
	}
}
func TestRerankTLSRedirectAndRedaction(t *testing.T) {
	tlsServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"results":[{"index":0,"relevance_score":0.8}]}`))
	}))
	defer tlsServer.Close()
	for _, insecure := range []bool{false, true} {
		e, _ := NewHTTPReranker(tlsServer.URL, "model", "", insecure, time.Second)
		defer e.Client.CloseIdleConnections()
		_, err := e.Rerank(context.Background(), "query", []Document{{Name: "tool"}})
		if (err == nil) != insecure {
			t.Fatalf("TLS opt-in ignored: %v", err)
		}
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer SECRET" {
			t.Error("missing provider credential")
		}
		w.WriteHeader(401)
		_, _ = w.Write([]byte(`{"error":{"code":"invalid_api_key","message":"SECRET PRIVATEQUERY PRIVATEDOC"}}`))
	}))
	defer srv.Close()
	e, _ := NewHTTPReranker(srv.URL+"/SECRET?key=SECRET", "model", "SECRET", false, time.Second)
	defer e.Client.CloseIdleConnections()
	_, err := e.Rerank(context.Background(), "PRIVATEQUERY", []Document{{Name: "PRIVATEDOC"}})
	var log bytes.Buffer
	slog.New(slog.NewJSONHandler(&log, nil)).Warn("failure", "error", err)
	if !strings.Contains(log.String(), `"http_status":401`) {
		t.Fatal(log.String())
	}
	for _, secret := range []string{"SECRET", "PRIVATEQUERY", "PRIVATEDOC"} {
		if strings.Contains(log.String(), secret) {
			t.Fatal("provider log leaked sensitive data")
		}
	}
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, tlsServer.URL, 302) }))
	defer redirect.Close()
	e, _ = NewHTTPReranker(redirect.URL, "model", "", true, time.Second)
	defer e.Client.CloseIdleConnections()
	if _, err = e.Rerank(context.Background(), "intent", []Document{{Name: "tool"}}); err == nil {
		t.Fatal("followed redirect")
	}
}
func TestRerankFreshACLNoMatchAndFailurePolicy(t *testing.T) {
	docs := []Document{{Ref: Ref("chat", "read"), Server: "chat", Name: "read", Description: "customer orders"}, {Ref: Ref("chat", "cancel"), Server: "chat", Name: "cancel", Description: "customer orders"}, {Ref: Ref("private", "secret"), Server: "private", Name: "secret", Description: "customer orders"}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _ = json.NewEncoder(w).Encode(Catalog{Documents: docs}) }))
	defer srv.Close()
	b, _ := NewBackend(srv.URL, strings.Repeat("x", 32))
	i := NewIndex(nil)
	i.Refresh(context.Background(), b)
	access := Access{Refs: map[string]string{docs[0].Ref: docs[0].Hash(), docs[1].Ref: docs[1].Hash()}}
	calls := 0
	i.Reranker = rerankFunc(func(ctx context.Context, q string, d []Document) ([]float64, error) {
		calls++
		if len(d) != 1 || d[0].Name != "cancel" {
			t.Fatalf("unauthorized or revoked metadata sent: %+v", d)
		}
		return []float64{0.95}, nil
	})
	ctx := context.WithValue(context.Background(), accessRefreshKey{}, accessRefresh(func(context.Context) (Access, error) {
		return Access{Refs: map[string]string{docs[1].Ref: docs[1].Hash()}}, nil
	}))
	hits, mode, err := i.Search(ctx, access, "orders", "", "", 1)
	if err != nil || len(hits) != 1 || hits[0].Name != "cancel" || mode != "keyword-reranked" || calls != 1 {
		t.Fatalf("%+v %s %v", hits, mode, err)
	}
	i.RerankMinScore = 0.96
	hits, _, err = i.Search(ctx, access, "orders", "", "", 1)
	if err != nil || len(hits) != 0 {
		t.Fatal("no-match threshold ignored")
	}
	i.Reranker = rerankFunc(func(context.Context, string, []Document) ([]float64, error) {
		return nil, errors.New("PRIVATE provider failure")
	})
	hits, mode, err = i.Search(ctx, access, "orders", "", "", 1)
	if err != nil || len(hits) != 1 || mode != "keyword-rerank-fallback" {
		t.Fatalf("fallback failed: %v %s %v", hits, mode, err)
	}
	i.RerankFailurePolicy = "closed"
	if _, _, err = i.Search(ctx, access, "orders", "", "", 1); err == nil {
		t.Fatal("closed policy failed open")
	}
	denied := context.WithValue(context.Background(), accessRefreshKey{}, accessRefresh(func(context.Context) (Access, error) { return Access{}, &HTTPError{503} }))
	i.Reranker = rerankFunc(func(context.Context, string, []Document) ([]float64, error) {
		t.Fatal("provider invoked while auth unavailable")
		return nil, nil
	})
	if _, _, err = i.Search(denied, access, "orders", "", "", 1); err == nil {
		t.Fatal("auth outage failed open")
	}
}
func TestRerankServerGroupingAndCancellation(t *testing.T) {
	i := NewIndex(nil)
	i.status.Ready = true
	access := Access{Refs: map[string]string{}}
	for _, d := range []Document{{Ref: Ref("a", "read"), Server: "a", Name: "read", Description: "orders"}, {Ref: Ref("a", "cancel"), Server: "a", Name: "cancel", Description: "orders"}, {Ref: Ref("b", "read"), Server: "b", Name: "read", Description: "orders"}} {
		i.entries = append(i.entries, prepareEntry(d))
		access.Refs[d.Ref] = d.Hash()
	}
	i.Reranker = rerankFunc(func(_ context.Context, _ string, d []Document) ([]float64, error) {
		if len(d) != 3 {
			t.Fatal("grouped servers before rerank")
		}
		scores := make([]float64, len(d))
		for n, v := range d {
			scores[n] = 0.2
			if v.Name == "cancel" {
				scores[n] = 0.9
			}
		}
		return scores, nil
	})
	hits, _, err := i.SearchServers(context.Background(), access, "orders", "", "", 2)
	if err != nil || len(hits) != 2 || hits[0].Server != "a" || hits[0].Name != "cancel" {
		t.Fatalf("grouping: %+v %v", hits, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	i.Reranker = rerankFunc(func(context.Context, string, []Document) ([]float64, error) { return nil, context.Canceled })
	if _, _, err = i.Search(ctx, access, "orders", "", "", 1); err == nil {
		t.Fatal("cancellation silently fell back")
	}
}

func TestRerankHTTPTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(100 * time.Millisecond):
		}
	}))
	defer srv.Close()
	e, _ := NewHTTPReranker(srv.URL, "model", "", false, 10*time.Millisecond)
	defer e.Client.CloseIdleConnections()
	_, err := e.Rerank(context.Background(), "intent", []Document{{Name: "tool"}})
	var diagnostic *embeddingError
	if !errors.As(err, &diagnostic) || diagnostic.reason != "timeout" {
		t.Fatalf("timeout cause lost: %v", err)
	}
}
func TestRerankHTTPServerChecksACLBeforeAndAfterProvider(t *testing.T) {
	for _, revokeAt := range []int{3, 4} {
		t.Run(string(rune('0'+revokeAt)), func(t *testing.T) {
			doc := Document{Ref: Ref("chat", "orders"), Server: "chat", Name: "orders", Description: "private order data"}
			calls := 0
			backendServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				a := Access{Refs: map[string]string{}}
				if calls < revokeAt {
					a.Refs[doc.Ref] = doc.Hash()
				}
				_ = json.NewEncoder(w).Encode(a)
			}))
			defer backendServer.Close()
			backend, _ := NewBackend(backendServer.URL, strings.Repeat("s", 32))
			index := NewIndex(nil)
			index.status.Ready = true
			index.entries = []entry{prepareEntry(doc)}
			modelCalls := 0
			index.Reranker = rerankFunc(func(context.Context, string, []Document) ([]float64, error) { modelCalls++; return []float64{0.9}, nil })
			handler := (&Server{Backend: backend, Index: index}).Handler()
			request := httptest.NewRequest("POST", "/mcp/v1/omni", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"discover_tools","arguments":{"query":"orders"}}}`))
			request.Header.Set("Authorization", "Bearer runtime")
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != 200 || strings.Contains(response.Body.String(), doc.Description) {
				t.Fatalf("revocation leaked result: %d %s", response.Code, response.Body)
			}
			if (revokeAt == 3 && modelCalls != 0) || (revokeAt == 4 && modelCalls != 1) {
				t.Fatalf("unexpected external metadata disclosure: revoke=%d calls=%d", revokeAt, modelCalls)
			}
		})
	}
}

func TestRerankQueryGateAndUncertainty(t *testing.T) {
	hits := []Hit{{Document: Document{Ref: Ref("chat", "read"), Name: "read"}, Score: 0.8}, {Document: Document{Ref: Ref("chat", "cancel"), Name: "cancel"}, Score: 0.7}}
	scores := []float64{.00001, .00002}
	i := NewIndex(nil)
	i.RerankNoMatchScore = .0001
	i.RerankConfidenceScore = .1
	i.Reranker = rerankFunc(func(context.Context, string, []Document) ([]float64, error) { return scores, nil })
	result, mode, err := i.refine(context.Background(), "intent", hits, 2, false, "hybrid")
	if err != nil || len(result) != 0 || mode != "hybrid-reranked-no-match" {
		t.Fatalf("query rejection: %v %s %v", result, mode, err)
	}
	scores = []float64{.03, .08}
	result, mode, err = i.refine(context.Background(), "intent", hits, 2, false, "hybrid")
	if err != nil || len(result) != 2 || result[0].Name != "read" || result[0].Score != .8 || mode != "hybrid-rerank-uncertain" {
		t.Fatalf("uncertain order: %v %s %v", result, mode, err)
	}
	scores = []float64{.7, .9}
	result, mode, err = i.refine(context.Background(), "intent", hits, 2, false, "hybrid")
	if err != nil || result[0].Name != "cancel" || mode != "hybrid-reranked" {
		t.Fatalf("confident rerank: %v %s %v", result, mode, err)
	}
	i.RerankMinScore = .95
	result, _, err = i.refine(context.Background(), "intent", hits, 2, false, "hybrid")
	if err != nil || len(result) != 0 {
		t.Fatal("per-tool threshold ignored")
	}
}

func TestRerankServerDiscoveryBalancesCandidatePool(t *testing.T) {
	i := NewIndex(nil)
	i.status.Ready = true
	i.RerankCandidates = 20
	access := Access{Refs: map[string]string{}}
	for n := 0; n < 30; n++ {
		d := Document{Ref: Ref("large", string(rune('A'+n))), Server: "large", Name: "orders", Description: "customer orders"}
		i.entries = append(i.entries, prepareEntry(d))
		access.Refs[d.Ref] = d.Hash()
	}
	d := Document{Ref: Ref("small", "orders"), Server: "small", Name: "orders"}
	i.entries = append(i.entries, prepareEntry(d))
	access.Refs[d.Ref] = d.Hash()
	i.Reranker = rerankFunc(func(_ context.Context, _ string, docs []Document) ([]float64, error) {
		scores := make([]float64, len(docs))
		found := false
		for n, d := range docs {
			scores[n] = .1
			if d.Server == "small" {
				found = true
				scores[n] = .9
			}
		}
		if !found || len(docs) > 4 {
			t.Fatalf("server diversity lost: %+v", docs)
		}
		return scores, nil
	})
	hits, _, err := i.SearchServers(context.Background(), access, "customer orders", "", "", 2)
	if err != nil || len(hits) != 2 || hits[0].Server != "small" {
		t.Fatalf("server discovery: %+v %v", hits, err)
	}
}
