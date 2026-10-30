package omni

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type captureAuditor struct {
	starts              []AuditStart
	finishes            []AuditFinish
	beginErr, finishErr error
}

func (a *captureAuditor) Begin(_ context.Context, _, _, _ string, p AuditStart) (AuditOperation, error) {
	a.starts = append(a.starts, p)
	return AuditOperation{ID: "operation", Ticket: "ticket", Options: AuditOptions{Details: true, Candidates: true, Limit: 2}}, a.beginErr
}
func (a *captureAuditor) Complete(_ context.Context, _, _, _ string, p AuditFinish) error {
	a.finishes = append(a.finishes, p)
	return a.finishErr
}
func TestOmniAuditsFourToolsAndStrictFailures(t *testing.T) {
	calls := 0
	dp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; _, _ = w.Write([]byte(`{"refs":{}}`)) }))
	defer dp.Close()
	backend, _ := NewBackend(dp.URL, strings.Repeat("s", 32))
	a := &captureAuditor{}
	s := &Server{Backend: backend, Index: NewIndex(nil), Auditor: a}
	handler := s.Handler()
	call := func(name string) *httptest.ResponseRecorder {
		raw, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": name, "arguments": map[string]any{"query": "tables", "tool_ref": "invalid"}}})
		r := httptest.NewRequest("POST", "/mcp/v1/omni", bytes.NewReader(raw))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Authorization", "Bearer runtime")
		out := httptest.NewRecorder()
		handler.ServeHTTP(out, r)
		return out
	}
	for _, name := range []string{"discover_server", "discover_tools", "get_tool_schema", "invoke_tool"} {
		call(name)
		if len(a.starts) != len(a.finishes) || a.starts[len(a.starts)-1].Tool != name {
			t.Fatalf("missing audit for %s", name)
		}
		if !json.Valid(a.starts[len(a.starts)-1].Payload) {
			t.Fatal("request payload lost")
		}
	}
	before := calls
	a.beginErr = errors.New("unavailable")
	if out := call("invoke_tool"); out.Code != 503 || calls != before {
		t.Fatal("audit admission failure dispatched operation")
	}
	a.beginErr = nil
	a.finishErr = errors.New("unavailable")
	if out := call("get_tool_schema"); out.Code != 503 || !strings.Contains(out.Body.String(), "do not automatically retry") {
		t.Fatalf("outcome failure returned success: %s", out.Body)
	}
}
func TestTraceBoundsAndProviderPayloadIndependence(t *testing.T) {
	trace := newDiscoveryTrace(AuditOptions{Candidates: true, Limit: 2})
	ctx := context.WithValue(context.Background(), discoveryTraceKey{}, trace)
	hits := []Hit{{Document: Document{Name: "one"}}, {Document: Document{Name: "two"}}, {Document: Document{Name: "three"}}}
	traceHits(ctx, "retrieval", hits)
	if trace.Stages[0].Details != nil {
		t.Fatal("candidate toggle bypassed detailed payload switch")
	}
	trace.Options.Details = true
	traceHits(ctx, "retrieval", hits)
	if trace.Stages[1].Omitted != 1 {
		t.Fatal("candidate sample limit ignored")
	}
	trace.Options.Details = false
	trace.Options.EmbeddingResponse = true
	traceProvider(ctx, "embedding", "response", []byte(`{"credential":"provider-secret","data":[1,2]}`), "provider-secret", 200)
	raw, _ := json.Marshal(trace)
	if bytes.Contains(raw, []byte("provider-secret")) {
		t.Fatal("model credential leaked")
	}
	if trace.Stages[2].Details == nil {
		t.Fatal("provider capture incorrectly depends on pipeline capture")
	}
	for i := 0; i < 100; i++ {
		trace.add(TraceStage{Name: "oversized", Count: 1, Details: strings.Repeat("x", 100000)})
	}
	if len(trace.Stages) > 32 || trace.bytes > 512<<10 {
		t.Fatal("unbounded trace")
	}
}

type auditReranker struct{}

func (auditReranker) Rerank(_ context.Context, _ string, d []Document) ([]float64, error) {
	scores := make([]float64, len(d))
	for i := range scores {
		scores[i] = 0.1
	}
	if len(scores) > 0 {
		scores[0] = 0.9
	}
	return scores, nil
}
func TestSearchTraceAuthorizationRerankAndNoLeak(t *testing.T) {
	idx := NewIndex(nil)
	idx.status.Ready = true
	access := Access{Refs: map[string]string{}}
	for _, name := range []string{"allowed-one", "allowed-two", "secret-tool"} {
		doc := Document{Ref: Ref("chat", name), Server: "chat", Name: name, Description: "customer accounts"}
		e := prepareEntry(doc)
		idx.entries = append(idx.entries, e)
		if name != "secret-tool" {
			access.Refs[e.Ref] = e.Hash
		}
	}
	idx.Reranker = auditReranker{}
	idx.RerankMinScore = 0.5
	trace := newDiscoveryTrace(AuditOptions{Details: true, Candidates: true, Limit: 10})
	ctx := context.WithValue(context.Background(), discoveryTraceKey{}, trace)
	hits, _, err := idx.Search(ctx, access, "customer accounts", "", "", 10)
	if err != nil || len(hits) != 1 || trace.Counts["unauthorized"] != 1 || trace.Counts["reranker_min_score_removed"] != 1 {
		t.Fatalf("trace %+v hits %v err %v", trace, hits, err)
	}
	raw, _ := json.Marshal(trace)
	if bytes.Contains(raw, []byte("secret-tool")) {
		t.Fatal("unauthorized identities captured without opt-in")
	}
}
