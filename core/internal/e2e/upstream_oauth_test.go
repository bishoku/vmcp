package e2e

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bishoku/vmcp/core/internal/fixture"
	"github.com/bishoku/vmcp/core/internal/resilience"
	"github.com/bishoku/vmcp/core/internal/upstream"
	"github.com/bishoku/vmcp/core/internal/virtual"
)

// TestUpstreamOAuth drives the gateway against an upstream that only accepts a bearer token the
// gateway must obtain itself from an OAuth token endpoint.
func TestUpstreamOAuth(t *testing.T) {
	e := setup(t)

	var fetches atomic.Int32
	var failing atomic.Bool
	idp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if failing.Load() {
			http.Error(w, "idp down", 500)
			return
		}
		if r.PostForm.Get("grant_type") != "client_credentials" || r.PostForm.Get("client_secret") != "gw-secret" && !strings.HasPrefix(r.Header.Get("Authorization"), "Basic ") {
			http.Error(w, "bad request", 400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"access_token":"gw-token-%d","token_type":"Bearer","expires_in":3600}`, fetches.Add(1))
	}))
	t.Cleanup(idp.Close)

	var withoutToken atomic.Int32
	inner := fixture.New(fixture.Options{})
	secured := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer gw-token-") {
			withoutToken.Add(1)
			http.Error(w, "unauthorized", 401)
			return
		}
		inner.ServeHTTP(w, r)
	}))
	t.Cleanup(secured.Close)

	sealed, err := e.syncer.Box.Seal([]byte("gw-secret"))
	if err != nil {
		t.Fatal(err)
	}
	e.exec(t, `INSERT INTO upstreams (tenant_id, slug, name, url, transport, oauth_token_url, oauth_client_id, oauth_scopes, oauth_audience, encrypted_oauth_secret)
		SELECT id, 'oa', 'OA', $1, 'streamable_http', $2, 'gateway', ARRAY['mcp.read'], 'fixture', $3 FROM tenants WHERE slug='default'`,
		secured.URL, idp.URL, sealed)
	for _, tool := range fixture.Tools() {
		e.exec(t, `INSERT INTO upstream_tools (upstream_id, name, description, input_schema)
			SELECT id, $1, $2, $3::jsonb FROM upstreams WHERE slug='oa'`, tool.Name, tool.Description, string(tool.InputSchema))
	}
	e.exec(t, `INSERT INTO virtual_servers (tenant_id, slug, name, auth_required)
		SELECT id, 'oauthvs', 'OAuth VS', false FROM tenants WHERE slug='default'`)
	e.exec(t, `INSERT INTO virtual_server_tools (virtual_server_id, upstream_id, tool_name)
		SELECT v.id, u.id, 'add' FROM virtual_servers v, upstreams u WHERE v.slug='oauthvs' AND u.slug='oa'`)

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	mux := http.NewServeMux()
	(&virtual.Handler{Snap: e.syncer, Sessions: virtual.NewManager(), Log: log, Breakers: resilience.NewRegistry(2, time.Hour)}).Register(mux)
	gw := httptest.NewServer(mux)
	t.Cleanup(gw.Close)
	url := gw.URL + "/mcp/v1/virtual/oauthvs"

	// The snapshot is pushed asynchronously.
	var first *upstream.Client
	for end := time.Now().Add(5 * time.Second); time.Now().Before(end); time.Sleep(50 * time.Millisecond) {
		c := upstream.NewClient(url, nil, nil)
		if _, err := c.Initialize(e.ctx); err == nil {
			if names, err := c.ListTools(e.ctx); err == nil && len(names) > 0 {
				first = c
				break
			}
		}
	}
	if first == nil {
		t.Fatal("virtual server never became available")
	}

	t.Run("one token shared by many sessions", func(t *testing.T) {
		var wg sync.WaitGroup
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				c := upstream.NewClient(url, nil, nil)
				if _, err := c.Initialize(e.ctx); err != nil {
					t.Error(err)
					return
				}
				defer c.Close(e.ctx)
				if got := call(t, c, "oa__add", `{"a":2,"b":3}`); got.IsError || got.Content[0].Text != "5" {
					t.Errorf("call: %+v", got)
				}
			}()
		}
		wg.Wait()
		if n := fetches.Load(); n != 1 {
			t.Fatalf("token endpoint called %d times for 8 sessions, want 1", n)
		}
		if withoutToken.Load() != 0 {
			t.Fatalf("%d upstream requests went out without a token", withoutToken.Load())
		}
	})

	t.Run("idp outage is not an upstream failure", func(t *testing.T) {
		// Force a fresh token fetch that fails.
		e.exec(t, `UPDATE upstreams SET oauth_client_id = 'gateway2' WHERE slug = 'oa'`)
		failing.Store(true)
		var failed bool
		for end := time.Now().Add(5 * time.Second); time.Now().Before(end) && !failed; time.Sleep(50 * time.Millisecond) {
			c := upstream.NewClient(url, nil, nil)
			if _, err := c.Initialize(e.ctx); err == nil {
				_, cerr := c.CallTool(e.ctx, "oa__add", []byte(`{"a":1,"b":1}`))
				failed = cerr != nil
				_ = c.Close(e.ctx)
			}
		}
		if !failed {
			t.Fatal("calls should fail while the token endpoint is down")
		}
		// More failures than the breaker threshold (2): the circuit must stay closed.
		for i := 0; i < 4; i++ {
			c := upstream.NewClient(url, nil, nil)
			if _, err := c.Initialize(e.ctx); err == nil {
				_, _ = c.CallTool(e.ctx, "oa__add", []byte(`{"a":1,"b":1}`))
				_ = c.Close(e.ctx)
			}
		}
		failing.Store(false)
		c := upstream.NewClient(url, nil, nil)
		if _, err := c.Initialize(e.ctx); err != nil {
			t.Fatal(err)
		}
		defer c.Close(e.ctx)
		var last error
		for end := time.Now().Add(2 * time.Second); time.Now().Before(end); time.Sleep(25 * time.Millisecond) {
			_, last = c.CallTool(e.ctx, "oa__add", []byte(`{"a":4,"b":4}`))
			if last == nil {
				return
			}
		}
		t.Fatalf("circuit did not recover after token failure backoff: %v", last)
	})
}
