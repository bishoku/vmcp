// Package e2e exercises the whole data path: Postgres -> snapshot -> gateway -> upstream.
package e2e

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/bishoku/vmcp/core/internal/fixture"
	"github.com/bishoku/vmcp/core/internal/mcp"
	"github.com/bishoku/vmcp/core/internal/secrets"
	"github.com/bishoku/vmcp/core/internal/snapshot"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/bishoku/vmcp/core/internal/audit"
	"github.com/bishoku/vmcp/core/internal/health"
	"github.com/bishoku/vmcp/core/internal/ratelimit"
	"github.com/bishoku/vmcp/core/internal/resilience"
	"github.com/bishoku/vmcp/core/internal/store"
	"github.com/bishoku/vmcp/core/internal/testdb"
	"github.com/bishoku/vmcp/core/internal/upstream"
	"github.com/bishoku/vmcp/core/internal/virtual"
)

type env struct {
	ctx    context.Context
	syncer *snapshot.Syncer
	gwURL  string
	pool   *pgxpool.Pool
}

func (e *env) exec(t testing.TB, q string, args ...any) {
	t.Helper()
	if _, err := e.syncer.Pool.Exec(e.ctx, q, args...); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
}

func setup(t testing.TB) *env {
	t.Helper()
	url := testdb.New(t)
	ctx, cancel := context.WithCancel(context.Background())
	if err := store.Migrate(ctx, url); err != nil {
		cancel()
		t.Fatal(err)
	}
	pool, err := store.Connect(ctx, url)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	// LIFO: cancel runs before pool.Close so the NOTIFY listener lets go of its connection.
	t.Cleanup(pool.Close)
	t.Cleanup(cancel)

	k := make([]byte, 32)
	_, _ = rand.Read(k)
	box, err := secrets.NewBox(base64.StdEncoding.EncodeToString(k))
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	syncer := &snapshot.Syncer{Pool: pool, Box: box, Log: log, PollInterval: time.Hour}
	auditCtx, auditCancel := context.WithCancel(ctx)
	aw := audit.NewWriter(auditCtx, pool, log)
	t.Cleanup(func() { auditCancel(); <-aw.Done() })
	e := &env{ctx: ctx, syncer: syncer, pool: pool}
	e.exec(t, `DELETE FROM virtual_servers`)
	e.exec(t, `DELETE FROM upstreams`)

	// Two real fixture servers: alpha needs a bearer token (JSON replies), beta streams SSE.
	alpha := httptest.NewServer(fixture.New(fixture.Options{Token: "fixture-secret"}))
	beta := httptest.NewServer(fixture.New(fixture.Options{SSE: true}))
	t.Cleanup(alpha.Close)
	t.Cleanup(beta.Close)

	enc, err := box.SealHeadersBound(map[string]string{"Authorization": "Bearer fixture-secret"}, "00000000-0000-4000-8000-000000000001")
	if err != nil {
		t.Fatal(err)
	}
	e.exec(t, `INSERT INTO upstreams (id, tenant_id, slug, name, url, transport, encrypted_headers)
		SELECT '00000000-0000-4000-8000-000000000001'::uuid, id, 'alpha', 'Alpha', $1, 'streamable_http', $2 FROM tenants WHERE slug='default'`, alpha.URL, enc)
	e.exec(t, `INSERT INTO upstreams (tenant_id, slug, name, url, transport)
		SELECT id, 'beta', 'Beta', $1, 'streamable_http' FROM tenants WHERE slug='default'`, beta.URL)

	// Cache the tool schemas exactly as the control plane will.
	for _, up := range []string{"alpha", "beta"} {
		for _, tool := range fixture.Tools() {
			e.exec(t, `INSERT INTO upstream_tools (upstream_id, name, description, input_schema)
				SELECT id, $2, $3, $4::jsonb FROM upstreams WHERE slug=$1`, up, tool.Name, tool.Description, string(tool.InputSchema))
		}
	}

	e.exec(t, `INSERT INTO virtual_servers (tenant_id, slug, name, auth_required)
		SELECT id, 'agent', 'Agent', false FROM tenants WHERE slug='default'`)
	pick := func(up, tool, alias string) {
		e.exec(t, `INSERT INTO virtual_server_tools (virtual_server_id, upstream_id, tool_name, alias)
			SELECT v.id, u.id, $2, NULLIF($3, '') FROM virtual_servers v, upstreams u WHERE v.slug='agent' AND u.slug=$1`, up, tool, alias)
	}
	pick("alpha", "echo", "say") // alias
	pick("alpha", "add", "")
	pick("alpha", "whoami", "")
	pick("alpha", "fail", "")
	pick("beta", "echo", "") // not exposed: whoami/add/fail of beta

	if err := syncer.Start(ctx); err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	(&virtual.Handler{Snap: syncer, Sessions: virtual.NewManager(), Log: log, Audit: aw}).Register(mux)
	gw := httptest.NewServer(mux)
	t.Cleanup(gw.Close)
	e.gwURL = gw.URL + "/mcp/v1/virtual/agent"
	return e
}

type toolResult struct {
	Content []struct {
		Text string `json:"text"`
	} `json:"content"`
	IsError bool `json:"isError"`
}

func call(t *testing.T, c *upstream.Client, name, args string) toolResult {
	t.Helper()
	raw, err := c.CallTool(context.Background(), name, json.RawMessage(args))
	if err != nil {
		t.Fatalf("call %s: %v", name, err)
	}
	var r toolResult
	if err := json.Unmarshal(raw, &r); err != nil || len(r.Content) == 0 {
		t.Fatalf("call %s: bad result %s (%v)", name, raw, err)
	}
	return r
}

func toolNames(t *testing.T, c *upstream.Client) []string {
	t.Helper()
	tools, err := c.ListTools(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(tools))
	for _, tl := range tools {
		names = append(names, tl.Name)
	}
	sort.Strings(names)
	return names
}

func TestEndToEnd(t *testing.T) {
	e := setup(t)
	ctx := context.Background()

	// The MCP client under test is the same one the gateway uses upstream.
	c := upstream.NewClient(e.gwURL, nil, nil)
	if _, err := c.Initialize(ctx); err != nil {
		t.Fatalf("initialize through gateway: %v", err)
	}
	t.Cleanup(func() { _ = c.Close(ctx) })

	t.Run("tool list is filtered, namespaced and aliased", func(t *testing.T) {
		got := toolNames(t, c)
		want := []string{"alpha__add", "alpha__fail", "alpha__whoami", "beta__echo", "say"}
		if len(got) != len(want) {
			t.Fatalf("got %v, want %v", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("got %v, want %v", got, want)
			}
		}
	})

	t.Run("alias routes to the original upstream tool", func(t *testing.T) {
		if r := call(t, c, "say", `{"message":"hello"}`); r.Content[0].Text != "hello" || r.IsError {
			t.Fatalf("unexpected: %+v", r)
		}
	})

	t.Run("arguments pass through", func(t *testing.T) {
		if r := call(t, c, "alpha__add", `{"a":2,"b":3}`); r.Content[0].Text != "5" {
			t.Fatalf("unexpected: %+v", r)
		}
	})

	t.Run("encrypted upstream header is decrypted and forwarded", func(t *testing.T) {
		if r := call(t, c, "alpha__whoami", `{}`); r.Content[0].Text != "Bearer fixture-secret" {
			t.Fatalf("upstream saw %q", r.Content[0].Text)
		}
	})

	t.Run("tool-level errors pass through untouched", func(t *testing.T) {
		if r := call(t, c, "alpha__fail", `{}`); !r.IsError || r.Content[0].Text != "intentional failure" {
			t.Fatalf("unexpected: %+v", r)
		}
	})

	t.Run("SSE upstream works and its notifications do not break the call", func(t *testing.T) {
		if r := call(t, c, "beta__echo", `{"message":"streamed"}`); r.Content[0].Text != "streamed" {
			t.Fatalf("unexpected: %+v", r)
		}
	})

	t.Run("tool not in the virtual server is rejected", func(t *testing.T) {
		_, err := c.CallTool(ctx, "beta__whoami", nil)
		var rpcErr *mcp.Error
		if !errors.As(err, &rpcErr) || rpcErr.Code != mcp.CodeInvalidParams {
			t.Fatalf("want invalid params, got %v", err)
		}
	})

	t.Run("config change propagates without restart", func(t *testing.T) {
		e.exec(t, `DELETE FROM virtual_server_tools WHERE tool_name='add'`)
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			names := toolNames(t, c)
			if len(names) == 4 {
				if _, err := c.CallTool(ctx, "alpha__add", json.RawMessage(`{"a":1,"b":1}`)); err == nil {
					t.Fatal("removed tool is still callable")
				}
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
		t.Fatal("tool removal did not reach the data plane")
	})

	t.Run("upstream auth is really enforced (control for the header test)", func(t *testing.T) {
		// Bypassing the gateway without the bearer token must fail, otherwise the
		// whoami check above would prove nothing.
		direct := upstream.NewClient(e.syncer.Snapshot().Upstreams[firstUpstreamID(t, e, "alpha")].URL, nil, &http.Client{Timeout: 5 * time.Second})
		if _, err := direct.Initialize(ctx); err == nil {
			t.Fatal("alpha accepted a request without its bearer token")
		}
	})
}

func firstUpstreamID(t *testing.T, e *env, slug string) string {
	t.Helper()
	for id, u := range e.syncer.Snapshot().Upstreams {
		if u.Slug == slug {
			return id
		}
	}
	t.Fatalf("upstream %q not in snapshot", slug)
	return ""
}

func TestAgentKeyLifecycle(t *testing.T) {
	e := setup(t)
	status := func(key string) int {
		req, _ := http.NewRequest("POST", e.gwURL, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		if key != "" {
			req.Header.Set("Authorization", "Bearer "+key)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	waitFor := func(key string, want int) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			if status(key) == want {
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
		t.Fatalf("status for %q never became %d (last %d)", key, want, status(key))
	}

	e.exec(t, `UPDATE virtual_servers SET auth_required = true WHERE slug = 'agent'`)
	e.exec(t, `INSERT INTO agent_keys (tenant_id, slug, key_hash, key_prefix)
		SELECT id, 'bot', $1, 'vmcp_e2e' FROM tenants WHERE slug = 'default'`, store.HashKey("vmcp_e2e_secret"))
	e.exec(t, `INSERT INTO agent_key_servers SELECT k.id, v.id FROM agent_keys k, virtual_servers v WHERE k.slug='bot' AND v.slug='agent'`)

	waitFor("", http.StatusUnauthorized)
	waitFor("vmcp_e2e_secret", http.StatusOK)

	e.exec(t, `UPDATE agent_keys SET revoked_at = now() WHERE slug = 'bot'`)
	waitFor("vmcp_e2e_secret", http.StatusUnauthorized)
}

func TestAuditTrail(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	c := upstream.NewClient(e.gwURL, nil, nil)
	if _, err := c.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	_, _ = c.CallTool(ctx, "alpha__add", json.RawMessage(`{"a":1,"b":2}`))
	_, _ = c.CallTool(ctx, "alpha__fail", json.RawMessage(`{}`))
	_, _ = c.CallTool(ctx, "nonexistent", json.RawMessage(`{}`))

	// Events are flushed by a background writer; poll for them.
	want := map[string]string{"alpha__add": "ok", "alpha__fail": "tool_error", "": "error"}
	deadline := time.Now().Add(5 * time.Second)
	for {
		got := map[string]string{}
		rows, err := e.pool.Query(ctx, `SELECT COALESCE(tool,''), status FROM audit_events WHERE method = 'tools/call'`)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var tool, status string
			if err := rows.Scan(&tool, &status); err != nil {
				t.Fatal(err)
			}
			got[tool] = status
		}
		rows.Close()
		if len(got) == len(want) {
			for k, v := range want {
				if got[k] != v {
					t.Fatalf("tool %s: status %q, want %q", k, got[k], v)
				}
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("audit events not written: %v", got)
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Arguments never reach the log.
	var n int
	if err := e.pool.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE reason LIKE '%"a"%'`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("payload leaked into audit log: n=%d err=%v", n, err)
	}
}

func TestPIIMasking(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	c := upstream.NewClient(e.gwURL, nil, nil)
	if _, err := c.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	echo := func(msg string) string {
		t.Helper()
		return call(t, c, "beta__echo", `{"message":"`+msg+`"}`).Content[0].Text
	}
	waitFor := func(check func() bool) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			if check() {
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
		t.Fatal("config change did not propagate")
	}

	if got := echo("mail ada@example.com"); !strings.Contains(got, "ada@example.com") {
		t.Fatalf("masking must be off by default: %q", got)
	}

	e.exec(t, `UPDATE virtual_servers SET pii_config = '{"enabled":true,"detectors":["email"],"customRules":[{"name":"emp","pattern":"EMP-[0-9]{6}"}]}'`)
	waitFor(func() bool { return !strings.Contains(echo("mail ada@example.com"), "ada@example.com") })
	got := echo("mail ada@example.com id EMP-123456 card 4111 1111 1111 1111")
	if !strings.Contains(got, "[REDACTED:EMAIL]") || !strings.Contains(got, "[REDACTED:emp]") {
		t.Fatalf("not masked: %q", got)
	}
	if !strings.Contains(got, "4111 1111 1111 1111") {
		t.Fatalf("card detector was not selected, value must stay: %q", got)
	}

	// With maskArguments the upstream itself never sees the value.
	e.exec(t, `UPDATE virtual_servers SET pii_config = '{"enabled":true,"detectors":["email"],"maskArguments":true}'`)
	waitFor(func() bool { return strings.Contains(echo("x ada@example.com"), "REDACTED") })
}

func TestToolPolicy(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	c := upstream.NewClient(e.gwURL, nil, nil)
	if _, err := c.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	waitFor := func(check func() bool) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			if check() {
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
		t.Fatal("config change did not propagate")
	}
	has := func(name string) bool { return slices.Contains(toolNames(t, c), name) }

	// Only alpha/add declares itself read-only.
	e.exec(t, `UPDATE upstream_tools SET annotations = '{"readOnlyHint":true}'::jsonb
		WHERE name = 'add' AND upstream_id = (SELECT id FROM upstreams WHERE slug='alpha')`)
	e.exec(t, `UPDATE virtual_servers SET tool_policy = '{"default":"read_only"}'`)
	waitFor(func() bool { return !has("say") })
	if got := toolNames(t, c); len(got) != 1 || got[0] != "alpha__add" {
		t.Fatalf("read_only must expose only annotated tools, got %v", got)
	}
	if _, err := c.CallTool(ctx, "say", json.RawMessage(`{"message":"x"}`)); err == nil {
		t.Fatal("blocked tool must not be callable")
	}

	// Per-tool override beats the default; deny hides even a read-only tool.
	e.exec(t, `UPDATE virtual_servers SET tool_policy = '{"default":"read_only","tools":{"say":"allow","alpha__add":"deny"}}'`)
	waitFor(func() bool { return has("say") })
	if has("alpha__add") {
		t.Fatalf("denied tool still listed: %v", toolNames(t, c))
	}
	if got := call(t, c, "say", `{"message":"hi"}`); got.IsError {
		t.Fatalf("allowed override failed: %+v", got)
	}
}

// Two data-plane nodes sharing only PostgreSQL: a session opened on one is usable on the other.
func TestSessionResumesOnAnotherNode(t *testing.T) {
	e := setup(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	node := func() (string, *virtual.Manager) {
		m := virtual.NewManager()
		m.Store = virtual.PGSessionStore{Pool: e.pool}
		mux := http.NewServeMux()
		(&virtual.Handler{Snap: e.syncer, Sessions: m, Log: log}).Register(mux)
		srv := httptest.NewServer(mux)
		t.Cleanup(srv.Close)
		return srv.URL + "/mcp/v1/virtual/agent", m
	}
	urlA, _ := node()
	urlB, mgrB := node()

	post := func(url, sid, body string) (*http.Response, string) {
		t.Helper()
		req, _ := http.NewRequest("POST", url, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		if sid != "" {
			req.Header.Set("Mcp-Session-Id", sid)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp, string(b)
	}
	resp, _ := post(urlA, "", `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`)
	sid := resp.Header.Get("Mcp-Session-Id")
	if sid == "" {
		t.Fatal("no session id")
	}
	// Warm both node caches while the lifecycle is pending.
	ping := `{"jsonrpc":"2.0","id":2,"method":"ping"}`
	if r, _ := post(urlB, sid, ping); r.StatusCode != 200 {
		t.Fatalf("pending ping: %s", r.Status)
	}
	if r, body := post(urlB, sid, `{"jsonrpc":"2.0","id":3,"method":"tools/list"}`); !strings.Contains(body, `"code":-32600`) {
		t.Fatalf("pending operation: %s %s", r.Status, body)
	}
	if r, _ := post(urlA, sid, `{"jsonrpc":"2.0","method":"notifications/initialized"}`); r.StatusCode != 202 {
		t.Fatalf("initialized on A: %s", r.Status)
	}
	var ready bool
	var caps string
	if err := e.pool.QueryRow(e.ctx, `SELECT initialized, client_capabilities::text FROM sessions WHERE id = $1`, sid).Scan(&ready, &caps); err != nil || !ready || caps != "{}" {
		t.Fatalf("persisted lifecycle: %v %s %v", ready, caps, err)
	}
	call := `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"alpha__add","arguments":{"a":2,"b":3}}}`
	if r, body := post(urlB, sid, call); r.StatusCode != 200 || !strings.Contains(body, "5") {
		t.Fatalf("node B could not resume the session: %d %s", r.StatusCode, body)
	}
	if mgrB.Len() != 1 {
		t.Fatalf("node B should now hold the session locally, has %d", mgrB.Len())
	}
	if r, _ := post(urlB, "no-such-session", call); r.StatusCode != 404 {
		t.Fatalf("unknown session must stay 404, got %d", r.StatusCode)
	}

	// Ending the session on one node ends it for all.
	req, _ := http.NewRequest("DELETE", urlA, nil)
	req.Header.Set("Mcp-Session-Id", sid)
	if r, err := http.DefaultClient.Do(req); err != nil || r.StatusCode != 204 {
		t.Fatalf("delete: %v %v", err, r)
	}
	if r, _ := post(urlB, sid, call); r.StatusCode != 404 {
		t.Fatalf("deleted session resumed: %d", r.StatusCode)
	}
}

func TestCircuitBreakerCutsOffDeadUpstream(t *testing.T) {
	e := setup(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	mux := http.NewServeMux()
	(&virtual.Handler{Snap: e.syncer, Sessions: virtual.NewManager(), Log: log, Breakers: resilience.NewRegistry(2, time.Hour)}).Register(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	url := srv.URL + "/mcp/v1/virtual/agent"

	post := func(sid, body string) (*http.Response, string) {
		t.Helper()
		req, _ := http.NewRequest("POST", url, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		if sid != "" {
			req.Header.Set("Mcp-Session-Id", sid)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp, string(b)
	}
	// Fresh sessions keep the breaker setup independent of cached connection state.
	open := func() string {
		resp, _ := post("", `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`)
		sid := resp.Header.Get("Mcp-Session-Id")
		_, _ = post(sid, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
		return sid
	}
	sid := open()
	call := `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"alpha__add","arguments":{"a":1,"b":1}}}`
	if _, body := post(sid, call); !strings.Contains(body, `"result"`) {
		t.Fatalf("healthy upstream failed: %s", body)
	}

	// Kill alpha: point it at a closed port and wait for the snapshot to follow.
	e.exec(t, `UPDATE upstreams SET url = 'http://127.0.0.1:1/' WHERE slug = 'alpha'`)
	deadline := time.Now().Add(5 * time.Second)
	var body string
	for time.Now().Before(deadline) {
		if _, body = post(open(), call); strings.Contains(body, "upstream unavailable") {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !strings.Contains(body, "upstream unavailable") {
		t.Fatalf("dead upstream not reported: %s", body)
	}
	post(open(), call) // second consecutive failure trips the breaker (threshold 2)
	if _, body = post(open(), call); !strings.Contains(body, "temporarily unavailable") {
		t.Fatalf("breaker should refuse without dialing: %s", body)
	}
	// Other upstreams are unaffected.
	if _, body = post(open(), `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"beta__echo","arguments":{"message":"hi"}}}`); !strings.Contains(body, `"result"`) {
		t.Fatalf("breaker must be per upstream: %s", body)
	}
}

func TestFailoverAndHealth(t *testing.T) {
	e := setup(t)
	var orig string
	if err := e.pool.QueryRow(e.ctx, `SELECT url FROM upstreams WHERE slug='alpha'`).Scan(&orig); err != nil {
		t.Fatal(err)
	}
	// The primary is dead; the fallback is the real server.
	e.exec(t, `UPDATE upstreams SET url = 'http://127.0.0.1:1/', fallback_urls = ARRAY[$1]::text[] WHERE slug = 'alpha'`, orig)

	// Health on the old URL can already be healthy; wait for configuration convergence
	// before asserting fallback behavior or dispatching against a pinned revision.
	awaitConsistency(t, func() bool {
		for _, up := range e.syncer.Snapshot().Upstreams {
			if up.Slug == "alpha" {
				return up.URL == "http://127.0.0.1:1/" && len(up.FallbackURLs) == 1
			}
		}
		return false
	})
	chk := &health.Checker{Pool: e.pool, Snap: e.syncer, Node: "test-node", Interval: time.Hour}
	state := func() (st, url string) {
		t.Helper()
		_ = e.pool.QueryRow(e.ctx, `SELECT h.state, h.url FROM upstream_health h JOIN upstreams u ON u.id = h.upstream_id
			WHERE u.slug='alpha' AND h.node='test-node'`).Scan(&st, &url)
		return
	}
	waitState := func(want string) string {
		t.Helper()
		deadline := time.Now().Add(8 * time.Second)
		for time.Now().Before(deadline) {
			chk.CheckOnce(e.ctx)
			if st, url := state(); st == want {
				return url
			}
			time.Sleep(100 * time.Millisecond)
		}
		st, _ := state()
		t.Fatalf("alpha never became %s (is %q)", want, st)
		return ""
	}

	if url := waitState("healthy"); url != orig {
		t.Fatalf("health probe should report the fallback that answered, got %q", url)
	}

	// Agents get through the fallback too, on a fresh session.
	c := upstream.NewClient(e.gwURL, nil, nil)
	if _, err := c.Initialize(e.ctx); err != nil {
		t.Fatal(err)
	}
	if got := call(t, c, "alpha__add", `{"a":2,"b":3}`); got.IsError || got.Content[0].Text != "5" {
		t.Fatalf("failover call failed: %+v", got)
	}

	// Remove the fallback: now the upstream is truly down.
	e.exec(t, `UPDATE upstreams SET fallback_urls = '{}' WHERE slug = 'alpha'`)
	waitState("unhealthy")
}

func TestRateLimit(t *testing.T) {
	e := setup(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	mux := http.NewServeMux()
	(&virtual.Handler{Snap: e.syncer, Sessions: virtual.NewManager(), Log: log, Limiter: ratelimit.NewLocal()}).Register(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	url := srv.URL + "/mcp/v1/virtual/agent"
	post := func(sid, body string) (*http.Response, string) {
		t.Helper()
		req, _ := http.NewRequest("POST", url, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		if sid != "" {
			req.Header.Set("Mcp-Session-Id", sid)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp, string(b)
	}
	resp, _ := post("", `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`)
	sid := resp.Header.Get("Mcp-Session-Id")
	_, _ = post(sid, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	call := `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"alpha__add","arguments":{"a":1,"b":1}}}`

	// No limit configured: everything passes.
	for i := 0; i < 5; i++ {
		if r, _ := post(sid, call); r.StatusCode != 200 {
			t.Fatalf("unlimited call %d: %d", i, r.StatusCode)
		}
	}

	e.exec(t, `UPDATE virtual_servers SET rate_limit_per_minute = 3`)
	deadline := time.Now().Add(5 * time.Second)
	var limited *http.Response
	var body string
	for time.Now().Before(deadline) && limited == nil {
		if r, b := post(sid, call); r.StatusCode == http.StatusTooManyRequests {
			limited, body = r, b
		}
		time.Sleep(20 * time.Millisecond)
	}
	if limited == nil {
		t.Fatal("virtual server limit never kicked in")
	}
	if ra := limited.Header.Get("Retry-After"); ra == "" || ra == "0" {
		t.Fatalf("Retry-After missing: %q", ra)
	}
	if !strings.Contains(body, "rate limit exceeded") || !strings.Contains(body, "-32029") {
		t.Fatalf("unexpected body: %s", body)
	}
	// Non-call requests are not charged.
	if r, _ := post(sid, `{"jsonrpc":"2.0","id":3,"method":"tools/list"}`); r.StatusCode != 200 {
		t.Fatalf("tools/list must not be limited: %d", r.StatusCode)
	}
}

func TestDescriptionOverride(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	e.exec(t, `INSERT INTO virtual_servers (tenant_id, slug, name, auth_required)
		SELECT id, 'other', 'Other', false FROM tenants WHERE slug='default'`)
	e.exec(t, `INSERT INTO virtual_server_tools (virtual_server_id, upstream_id, tool_name)
		SELECT v.id, u.id, 'echo' FROM virtual_servers v, upstreams u WHERE v.slug='other' AND u.slug='alpha'`)
	e.exec(t, `UPDATE virtual_server_tools SET description = 'Say something to the user'
		WHERE alias = 'say'`)

	desc := func(url, name string) string {
		t.Helper()
		c := upstream.NewClient(url, nil, nil)
		if _, err := c.Initialize(ctx); err != nil {
			t.Fatal(err)
		}
		tools, err := c.ListTools(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, tl := range tools {
			if tl.Name == name {
				return tl.Description
			}
		}
		t.Fatalf("tool %q not listed", name)
		return ""
	}
	other := strings.TrimSuffix(e.gwURL, "/agent") + "/other"
	deadline := time.Now().Add(5 * time.Second)
	for desc(e.gwURL, "say") != "Say something to the user" {
		if time.Now().After(deadline) {
			t.Fatal("override never propagated")
		}
		time.Sleep(50 * time.Millisecond)
	}
	// The same upstream tool in another virtual server keeps the upstream text.
	if got := desc(other, "alpha__echo"); got != "Returns the given message" {
		t.Fatalf("override leaked into another virtual server: %q", got)
	}
}
