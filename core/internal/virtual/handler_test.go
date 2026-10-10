package virtual

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bishoku/vmcp/core/internal/config"
	"github.com/bishoku/vmcp/core/internal/mcp"
)

// fakeUpstream is a tiny Streamable HTTP MCP server whose tools reply "<name>:<tool>".
type fakeUpstream struct {
	name     string
	mu       sync.Mutex
	session  string
	inits    int
	deleted  atomic.Bool
	failCall bool
}

func (f *fakeUpstream) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodDelete {
		f.deleted.Store(true)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	body, _ := io.ReadAll(r.Body)
	m, err := mcp.Decode(body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if m.Method != mcp.MethodInitialize && r.Header.Get("Mcp-Session-Id") != f.session {
		http.Error(w, "bad session", http.StatusNotFound)
		return
	}
	if m.IsNotification() {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	var resp *mcp.Message
	switch m.Method {
	case mcp.MethodInitialize:
		f.inits++
		f.session = fmt.Sprintf("%s-%d", f.name, f.inits)
		w.Header().Set("Mcp-Session-Id", f.session)
		resp, _ = mcp.NewResult(m.ID, mcp.InitializeResult{ProtocolVersion: mcp.ProtocolVersion20250618})
	case mcp.MethodToolsCall:
		var p mcp.CallToolParams
		_ = json.Unmarshal(m.Params, &p)
		if f.failCall {
			resp = mcp.NewErrorResponse(m.ID, -32000, "boom")
			break
		}
		resp, _ = mcp.NewResult(m.ID, map[string]any{
			"content": []map[string]string{{"type": "text", "text": f.name + ":" + p.Name}},
		})
	default:
		resp = mcp.NewErrorResponse(m.ID, mcp.CodeMethodNotFound, "nope")
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

type rig struct {
	gw       *httptest.Server
	handler  *Handler
	github   *fakeUpstream
	postgres *fakeUpstream
}

func newRig(t *testing.T) *rig {
	t.Helper()
	gh, pg := &fakeUpstream{name: "gh"}, &fakeUpstream{name: "pg"}
	ghSrv, pgSrv := httptest.NewServer(gh), httptest.NewServer(pg)
	t.Cleanup(ghSrv.Close)
	t.Cleanup(pgSrv.Close)

	schema := json.RawMessage(`{"type":"object"}`)
	snap := &config.Snapshot{
		Revision: 1,
		Upstreams: map[string]config.Upstream{
			"u-gh": {ID: "u-gh", Slug: "github", URL: ghSrv.URL},
			"u-pg": {ID: "u-pg", Slug: "postgres", URL: pgSrv.URL},
		},
		VirtualServers: map[string]config.VirtualServer{
			"support": {Slug: "support", Name: "Support", Tools: []config.VirtualTool{
				{UpstreamID: "u-gh", UpstreamName: "create_issue", Tool: mcp.Tool{Name: config.ExposedName("github", "create_issue", ""), InputSchema: schema}},
				{UpstreamID: "u-pg", UpstreamName: "list_tables", Tool: mcp.Tool{Name: config.ExposedName("postgres", "list_tables", "tables"), InputSchema: schema}},
			}},
		},
	}
	h := &Handler{Snap: staticSnap{snap}, Sessions: NewManager(), Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	mux := http.NewServeMux()
	h.Register(mux)
	gw := httptest.NewServer(mux)
	t.Cleanup(gw.Close)
	return &rig{gw: gw, handler: h, github: gh, postgres: pg}
}

type staticSnap struct{ s *config.Snapshot }

func (s staticSnap) Snapshot() *config.Snapshot { return s.s }

func (r *rig) rpc(t *testing.T, slug, sid, method string, params any) (*http.Response, *mcp.Message) {
	t.Helper()
	req, _ := mcp.NewRequest(mcp.NewIntID(1), method, params)
	b, _ := json.Marshal(req)
	hr, _ := http.NewRequest(http.MethodPost, r.gw.URL+"/mcp/v1/virtual/"+slug, bytes.NewReader(b))
	hr.Header.Set("Content-Type", "application/json")
	if sid != "" {
		hr.Header.Set("Mcp-Session-Id", sid)
	}
	resp, err := http.DefaultClient.Do(hr)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	m, err := mcp.Decode(data)
	if err != nil {
		return resp, nil
	}
	return resp, m
}

func (r *rig) initialize(t *testing.T) string {
	t.Helper()
	resp, m := r.rpc(t, "support", "", mcp.MethodInitialize, mcp.InitializeParams{ProtocolVersion: mcp.ProtocolVersion20250618})
	sid := resp.Header.Get("Mcp-Session-Id")
	if sid == "" || m == nil || m.Error != nil {
		t.Fatalf("initialize failed: %v %+v", resp.Status, m)
	}
	return sid
}

func callText(t *testing.T, m *mcp.Message) string {
	t.Helper()
	if m == nil || m.Error != nil {
		t.Fatalf("call failed: %+v", m)
	}
	var res struct {
		Content []struct{ Text string } `json:"content"`
	}
	if err := json.Unmarshal(m.Result, &res); err != nil || len(res.Content) == 0 {
		t.Fatalf("bad result %s: %v", m.Result, err)
	}
	return res.Content[0].Text
}

func TestListToolsAndRouting(t *testing.T) {
	r := newRig(t)
	sid := r.initialize(t)

	_, m := r.rpc(t, "support", sid, mcp.MethodToolsList, nil)
	var list mcp.ListToolsResult
	_ = json.Unmarshal(m.Result, &list)
	if len(list.Tools) != 2 || list.Tools[0].Name != "github__create_issue" || list.Tools[1].Name != "tables" {
		t.Fatalf("unexpected tools: %+v", list.Tools)
	}

	_, m = r.rpc(t, "support", sid, mcp.MethodToolsCall, mcp.CallToolParams{Name: "github__create_issue"})
	if got := callText(t, m); got != "gh:create_issue" {
		t.Fatalf("routed wrong: %s", got)
	}
	_, m = r.rpc(t, "support", sid, mcp.MethodToolsCall, mcp.CallToolParams{Name: "tables"})
	if got := callText(t, m); got != "pg:list_tables" { // alias resolves to the original upstream tool name
		t.Fatalf("alias routed wrong: %s", got)
	}
}

func TestUpstreamsConnectLazily(t *testing.T) {
	r := newRig(t)
	sid := r.initialize(t)
	_, _ = r.rpc(t, "support", sid, mcp.MethodToolsCall, mcp.CallToolParams{Name: "github__create_issue"})

	if r.github.inits != 1 || r.postgres.inits != 0 {
		t.Fatalf("expected only github to be initialized, got gh=%d pg=%d", r.github.inits, r.postgres.inits)
	}
}

func TestRejections(t *testing.T) {
	r := newRig(t)
	sid := r.initialize(t)

	if resp, _ := r.rpc(t, "support", "", mcp.MethodToolsList, nil); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("missing session: want 400, got %d", resp.StatusCode)
	}
	if resp, _ := r.rpc(t, "support", "bogus", mcp.MethodToolsList, nil); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown session: want 404, got %d", resp.StatusCode)
	}
	if resp, _ := r.rpc(t, "nope", "", mcp.MethodInitialize, mcp.InitializeParams{}); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown slug: want 404, got %d", resp.StatusCode)
	}
	if _, m := r.rpc(t, "support", sid, mcp.MethodToolsCall, mcp.CallToolParams{Name: "not_exposed"}); m.Error == nil || m.Error.Code != mcp.CodeInvalidParams {
		t.Fatalf("unexposed tool must be rejected, got %+v", m)
	}
	if _, m := r.rpc(t, "support", sid, "resources/list", nil); m.Error == nil || m.Error.Code != mcp.CodeMethodNotFound {
		t.Fatalf("unsupported method: %+v", m)
	}
}

func TestUpstreamSessionRecovery(t *testing.T) {
	r := newRig(t)
	sid := r.initialize(t)
	_, _ = r.rpc(t, "support", sid, mcp.MethodToolsCall, mcp.CallToolParams{Name: "github__create_issue"})

	r.github.mu.Lock()
	r.github.session = "invalidated"
	r.github.mu.Unlock()

	_, m := r.rpc(t, "support", sid, mcp.MethodToolsCall, mcp.CallToolParams{Name: "github__create_issue"})
	if got := callText(t, m); got != "gh:create_issue" {
		t.Fatalf("recovery failed: %s", got)
	}
	if r.github.inits != 2 {
		t.Fatalf("expected a re-initialize, inits=%d", r.github.inits)
	}
}

func TestUpstreamErrorRelayedAndHiddenFailure(t *testing.T) {
	r := newRig(t)
	sid := r.initialize(t)
	r.github.failCall = true
	_, m := r.rpc(t, "support", sid, mcp.MethodToolsCall, mcp.CallToolParams{Name: "github__create_issue"})
	if m.Error == nil || m.Error.Code != -32000 || m.Error.Message != "boom" {
		t.Fatalf("upstream rpc error should be relayed verbatim: %+v", m)
	}
}

func TestDeleteClosesUpstreamSessions(t *testing.T) {
	r := newRig(t)
	sid := r.initialize(t)
	_, _ = r.rpc(t, "support", sid, mcp.MethodToolsCall, mcp.CallToolParams{Name: "github__create_issue"})

	req, _ := http.NewRequest(http.MethodDelete, r.gw.URL+"/mcp/v1/virtual/support", nil)
	req.Header.Set("Mcp-Session-Id", sid)
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: %v %v", err, resp)
	}
	deadline := time.Now().Add(2 * time.Second)
	for !r.github.deleted.Load() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !r.github.deleted.Load() {
		t.Fatal("upstream session was not closed")
	}
	if r.handler.Sessions.Len() != 0 {
		t.Fatal("session still registered")
	}
}

func TestReapIdleSessions(t *testing.T) {
	m := NewManager()
	s, _ := m.Create("support", mcp.ProtocolVersion20250618)
	s.mu.Lock()
	s.last = time.Now().Add(-time.Hour)
	s.mu.Unlock()
	fresh, _ := m.Create("support", mcp.ProtocolVersion20250618)

	if n := m.Reap(time.Minute); n != 1 {
		t.Fatalf("reaped %d, want 1", n)
	}
	if m.Get(s.id) != nil || m.Get(fresh.id) == nil {
		t.Fatal("wrong session reaped")
	}
}
