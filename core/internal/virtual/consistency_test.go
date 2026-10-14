package virtual

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bishoku/vmcp/core/internal/config"
	"github.com/bishoku/vmcp/core/internal/mcp"
	"github.com/bishoku/vmcp/core/internal/oauthtoken"
	"github.com/bishoku/vmcp/core/internal/tlsconf"
	"github.com/bishoku/vmcp/core/internal/upstream"
)

type memorySessions struct {
	mu                 sync.Mutex
	records            map[string]Record
	tombstones         map[string]bool
	loadErr, deleteErr error
	loads              atomic.Int32
}

func newMemorySessions() *memorySessions {
	return &memorySessions{records: map[string]Record{}, tombstones: map[string]bool{}}
}
func (s *memorySessions) Save(_ context.Context, r Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.tombstones[r.ID] {
		s.records[r.ID] = r
	}
	return nil
}
func (s *memorySessions) Load(_ context.Context, id string) (Record, bool, error) {
	s.loads.Add(1)
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.records[id]
	return r, ok && !s.tombstones[id], s.loadErr
}
func (s *memorySessions) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.deleteErr != nil {
		return s.deleteErr
	}
	s.tombstones[id] = true
	return nil
}
func (s *memorySessions) MarkInitialized(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.records[id]
	if !ok || s.tombstones[id] {
		return errors.New("missing")
	}
	r.Initialized = true
	s.records[id] = r
	return nil
}
func (*memorySessions) Touch(context.Context, string) error                 { return nil }
func (*memorySessions) Purge(context.Context, time.Duration) (int64, error) { return 0, nil }

func TestWarmSessionAuthoritativeTerminationAndFailures(t *testing.T) {
	ctx := context.Background()
	store := newMemorySessions()
	a, b := NewManager(), NewManager()
	a.Store = store
	b.Store = store
	s, err := a.Create("agent", mcp.ProtocolVersion20250618)
	if err != nil {
		t.Fatal(err)
	}
	s.owner = Owner{Mode: "oauth", Principal: "oauth:binding", CredentialID: "binding-id", Issuer: "https://issuer", Subject: "alice"}
	s.keySlug = s.owner.Principal
	if err := a.Persist(ctx, s); err != nil {
		t.Fatal(err)
	}
	warm, err := b.Resolve(ctx, s.id)
	if err != nil || warm == nil || warm.owner != s.owner {
		t.Fatalf("resolve: %v %+v", err, warm)
	}
	store.loadErr = errors.New("DB down")
	if got, err := b.Resolve(ctx, s.id); got != nil || err == nil {
		t.Fatal("warm cache bypassed store outage")
	}
	store.loadErr = nil
	store.deleteErr = errors.New("write failed")
	if err := a.End(ctx, s.id); err == nil || a.Get(s.id) == nil {
		t.Fatal("DELETE failure concealed or locally committed")
	}
	store.deleteErr = nil
	if err := a.End(ctx, s.id); err != nil {
		t.Fatal(err)
	}
	if got, err := b.Resolve(ctx, s.id); err != nil || got != nil || b.Get(s.id) != nil {
		t.Fatal("warm cache survived authoritative tombstone")
	}
	if err := store.Save(ctx, Record{ID: s.id}); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := store.Load(ctx, s.id); ok {
		t.Fatal("save resurrected terminated session")
	}
}

type closeConn struct{ closed chan struct{} }

func (*closeConn) Initialize(context.Context) (*mcp.InitializeResult, error) { return nil, nil }
func (*closeConn) ListTools(context.Context) ([]mcp.Tool, error)             { return nil, nil }
func (*closeConn) CallTool(context.Context, string, json.RawMessage) (json.RawMessage, error) {
	return nil, nil
}
func (c *closeConn) Close(context.Context) error { close(c.closed); return nil }
func awaitClosed(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatal("retired connection not closed")
	}
}

func TestRotationPreservesInFlightLeaseAndClosesIdleClient(t *testing.T) {
	m := NewManager()
	s, _ := m.Create("agent", mcp.ProtocolVersion20250618)
	up := config.Upstream{ID: "u", URL: "https://example.com/mcp", Headers: map[string]string{"Authorization": "old"}}
	old := &closeConn{closed: make(chan struct{})}
	uc := s.conn("u")
	uc.current = &cachedClient{conn: old, generation: up.Fingerprint()}
	_, release, err := s.client(context.Background(), up)
	if err != nil {
		t.Fatal(err)
	}
	up.Headers = map[string]string{"Authorization": "new"}
	m.Reconcile(&config.Snapshot{Upstreams: map[string]config.Upstream{"u": up}})
	select {
	case <-old.closed:
		t.Fatal("rotation closed a leased call")
	default:
	}
	if uc.current != nil {
		t.Fatal("superseded connection is reusable")
	}
	release()
	release()
	awaitClosed(t, old.closed)
	idle := &closeConn{closed: make(chan struct{})}
	uc.current = &cachedClient{conn: idle, generation: up.Fingerprint()}
	m.Reconcile(&config.Snapshot{Upstreams: map[string]config.Upstream{}})
	awaitClosed(t, idle.closed)
	m.Delete(s.id)
	if _, _, err := s.client(context.Background(), up); err == nil {
		t.Fatal("ended session admitted a new connection")
	}
}

func TestFingerprintCoversConnectionConfiguration(t *testing.T) {
	base := config.Upstream{ID: "u", URL: "https://example.com", Headers: map[string]string{"a": "1", "b": "2"}}
	reordered := base
	reordered.Headers = map[string]string{"b": "2", "a": "1"}
	if base.Fingerprint() != reordered.Fingerprint() {
		t.Fatal("map order changed generation")
	}
	for name, change := range map[string]func(*config.Upstream){
		"url":       func(u *config.Upstream) { u.URL += "/new" },
		"fallback":  func(u *config.Upstream) { u.FallbackURLs = []string{"https://other.example"} },
		"transport": func(u *config.Upstream) { u.Transport = "sse" },
		"headers":   func(u *config.Upstream) { u.Headers = map[string]string{"a": "new"} },
		"tls":       func(u *config.Upstream) { u.TLS = tlsconf.Settings{ClientKeyPEM: "rotated"} },
		"oauth":     func(u *config.Upstream) { u.OAuth = &oauthtoken.Config{ClientSecret: "rotated"} },
	} {
		t.Run(name, func(t *testing.T) {
			next := base
			change(&next)
			if next.Fingerprint() == base.Fingerprint() {
				t.Fatal("change did not invalidate client")
			}
		})
	}
}

type checkedSnapshot struct {
	snap  atomic.Pointer[config.Snapshot]
	stale atomic.Bool
}

func (s *checkedSnapshot) Snapshot() *config.Snapshot { return s.snap.Load() }
func (s *checkedSnapshot) AuthorizedSnapshot() (*config.Snapshot, error) {
	if s.stale.Load() {
		return nil, errors.New("stale")
	}
	return s.snap.Load(), nil
}

func TestStaleSnapshotDeniesEveryMCPMethod(t *testing.T) {
	r := newRig(t)
	provider := &checkedSnapshot{}
	provider.snap.Store(r.handler.Snap.Snapshot())
	r.handler.Snap = provider
	sid := r.initialize(t)
	provider.stale.Store(true)
	for _, method := range []string{"POST", "GET", "DELETE"} {
		t.Run(method, func(t *testing.T) {
			req, _ := http.NewRequest(method, r.gw.URL+"/mcp/v1/virtual/support", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json, text/event-stream")
			req.Header.Set("Mcp-Session-Id", sid)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != 503 {
				t.Fatalf("stale %s: %d", method, resp.StatusCode)
			}
		})
	}
}

func TestAuthorizationChangeDuringDialPreventsToolDispatch(t *testing.T) {
	for _, mode := range []string{"revision", "stale", "terminated"} {
		t.Run(mode, func(t *testing.T) {
			r := newRig(t)
			provider := &checkedSnapshot{}
			original := r.handler.Snap.Snapshot()
			provider.snap.Store(original)
			r.handler.Snap = provider
			var calls atomic.Int32
			var sid string
			fake := &fakeUpstream{name: "rotating"}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				b, _ := io.ReadAll(req.Body)
				req.Body = io.NopCloser(strings.NewReader(string(b)))
				var msg mcp.Message
				_ = json.Unmarshal(b, &msg)
				if msg.Method == mcp.MethodInitialize {
					switch mode {
					case "revision":
						next := *original
						next.Revision++
						provider.snap.Store(&next)
					case "stale":
						provider.stale.Store(true)
					case "terminated":
						r.handler.Sessions.Delete(sid)
					}
				}
				if msg.Method == mcp.MethodToolsCall {
					calls.Add(1)
				}
				fake.ServeHTTP(w, req)
			}))
			defer server.Close()
			original.Upstreams["u-gh"] = config.Upstream{ID: "u-gh", URL: server.URL}
			sid = r.initialize(t)
			resp, _ := r.rpc(t, "support", sid, mcp.MethodToolsCall, mcp.CallToolParams{Name: "github__create_issue", Arguments: json.RawMessage(`{}`)})
			want := map[string]int{"revision": 409, "stale": 503, "terminated": 404}[mode]
			if resp.StatusCode != want || calls.Load() != 0 {
				t.Fatalf("stale tool dispatch: status=%d calls=%d", resp.StatusCode, calls.Load())
			}
		})
	}
}

func TestAuthorizationChangeStopsIdempotentRetry(t *testing.T) {
	r := newRig(t)
	provider := &checkedSnapshot{}
	original := r.handler.Snap.Snapshot()
	provider.snap.Store(original)
	r.handler.Snap = provider
	var calls atomic.Int32
	fake := &fakeUpstream{name: "retrying"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		b, _ := io.ReadAll(req.Body)
		req.Body = io.NopCloser(strings.NewReader(string(b)))
		var msg mcp.Message
		_ = json.Unmarshal(b, &msg)
		if msg.Method == mcp.MethodToolsCall {
			calls.Add(1)
			next := *original
			next.Revision++
			provider.snap.Store(&next)
			conn, _, err := w.(http.Hijacker).Hijack()
			if err == nil {
				_ = conn.Close()
			}
			return
		}
		fake.ServeHTTP(w, req)
	}))
	defer server.Close()
	original.Upstreams["u-gh"] = config.Upstream{ID: "u-gh", URL: server.URL}
	vs := original.VirtualServers["support"]
	vs.Tools[0].Tool.Annotations = json.RawMessage(`{"idempotentHint":true}`)
	original.VirtualServers["support"] = vs
	sid := r.initialize(t)
	resp, _ := r.rpc(t, "support", sid, mcp.MethodToolsCall, mcp.CallToolParams{Name: "github__create_issue", Arguments: json.RawMessage(`{}`)})
	if resp.StatusCode != 409 || calls.Load() != 1 {
		t.Fatalf("unauthorized retry: status=%d calls=%d", resp.StatusCode, calls.Load())
	}
}

func TestDeleteStoreFailureIsVisible(t *testing.T) {
	r := newRig(t)
	store := newMemorySessions()
	r.handler.Sessions.Store = store
	sid := r.initialize(t)
	store.deleteErr = errors.New("database unavailable")
	req, _ := http.NewRequest("DELETE", r.gw.URL+"/mcp/v1/virtual/support", nil)
	req.Header.Set("Mcp-Session-Id", sid)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 503 || r.handler.Sessions.Get(sid) == nil {
		t.Fatalf("false termination success: %d", resp.StatusCode)
	}
}

func TestOwnerDistinguishesAuthenticationModesAndCredentialIDs(t *testing.T) {
	a := Owner{Mode: "agent-key", Principal: "same", CredentialID: "id-1"}
	for _, b := range []Owner{
		{Mode: "agent-key", Principal: "same", CredentialID: "id-2"},
		{Mode: "anonymous"},
		{Mode: "legacy", Principal: "same", CredentialID: "id-1"},
		{Mode: "oauth", Principal: "same", CredentialID: "id-1"},
	} {
		if a == b {
			t.Fatalf("distinct owner compares equal: %+v", b)
		}
	}
}

func TestReconcilePreservesClientCapabilityConnectionsWhenUpstreamUnchanged(t *testing.T) {
	fake := &fakeUpstream{name: "cap-upstream"}
	srv := httptest.NewServer(fake)
	defer srv.Close()

	m := NewManager()
	s, err := m.Create("agent", mcp.ProtocolVersion20250618)
	if err != nil {
		t.Fatal(err)
	}
	s.clientCapabilities = json.RawMessage(`{"sampling":{},"roots":{"listChanged":true}}`)
	vs := config.VirtualServer{
		Slug:           "agent",
		ClientFeatures: config.ClientFeatures{Sampling: true, Roots: true},
	}
	up := config.Upstream{ID: "u1", Slug: "cap", URL: srv.URL}.WithPrecomputedFingerprint()
	ctx := upstream.WithRelay(context.Background(), upstream.Relay{
		ClientCapabilities: vs.ClientFeatures.Negotiated(s.clientCapabilities),
	})
	_, release, err := s.client(ctx, up)
	if err != nil {
		t.Fatal(err)
	}
	release()

	uc := s.conn("u1")
	if uc.current == nil {
		t.Fatal("expected cached upstream client")
	}
	// Reconcile when an unrelated upstream changes; u1 and vs.ClientFeatures are unchanged.
	m.Reconcile(&config.Snapshot{
		Upstreams: map[string]config.Upstream{
			"u1": up,
			"u2": {ID: "u2", URL: "https://other.example/mcp"},
		},
		VirtualServers: map[string]config.VirtualServer{"agent": vs},
	})
	uc.mu.Lock()
	preserved := uc.current != nil && !uc.current.retired
	uc.mu.Unlock()
	if !preserved {
		t.Fatal("Reconcile retired unchanged capability-enabled upstream connection")
	}

	// Now disable Sampling on the virtual server; Reconcile must retire the connection.
	vs.ClientFeatures.Sampling = false
	m.Reconcile(&config.Snapshot{
		Upstreams:      map[string]config.Upstream{"u1": up},
		VirtualServers: map[string]config.VirtualServer{"agent": vs},
	})
	uc.mu.Lock()
	retired := uc.current == nil
	uc.mu.Unlock()
	if !retired {
		t.Fatal("Reconcile did not retire connection when negotiated client features changed")
	}
}

func TestWarmToolCallMinimizesSessionStoreLoads(t *testing.T) {
	r := newRig(t)
	store := newMemorySessions()
	r.handler.Sessions.Store = store
	sid := r.initialize(t)

	// Warm the upstream connection on the first call.
	resp, _ := r.rpc(t, "support", sid, mcp.MethodToolsCall, mcp.CallToolParams{Name: "github__create_issue", Arguments: json.RawMessage(`{"title":"warm"}`)})
	if resp.StatusCode != 200 {
		t.Fatalf("warm call status: %d", resp.StatusCode)
	}
	store.loads.Store(0)

	// A subsequent warm tool call should resolve the session from the store only once.
	resp, _ = r.rpc(t, "support", sid, mcp.MethodToolsCall, mcp.CallToolParams{Name: "github__create_issue", Arguments: json.RawMessage(`{"title":"hot"}`)})
	if resp.StatusCode != 200 {
		t.Fatalf("hot call status: %d", resp.StatusCode)
	}
	if got := store.loads.Load(); got != 1 {
		t.Fatalf("expected 1 session store Load on warm tool call, got %d", got)
	}
}

func TestVirtualServerToolIndexAndPrecomputedFingerprint(t *testing.T) {
	up := config.Upstream{
		ID:      "u-1",
		Slug:    "gh",
		URL:     "https://api.example.com/mcp",
		Headers: map[string]string{"Authorization": "Bearer secret"},
	}
	pre := up.WithPrecomputedFingerprint()
	if pre.Fingerprint() != up.Fingerprint() {
		t.Fatal("precomputed fingerprint mismatch")
	}
	vs := config.VirtualServer{
		Slug: "dev",
		Tools: []config.VirtualTool{
			{UpstreamID: "u-1", UpstreamName: "a", Tool: mcp.Tool{Name: "gh__a"}},
			{UpstreamID: "u-1", UpstreamName: "b", Tool: mcp.Tool{Name: "gh__b"}},
		},
	}.WithToolIndex()
	if vt, ok := vs.Lookup("gh__b"); !ok || vt.UpstreamName != "b" {
		t.Fatalf("indexed lookup failed: %+v %v", vt, ok)
	}
	if _, ok := vs.Lookup("gh__missing"); ok {
		t.Fatal("indexed lookup found nonexistent tool")
	}
}
