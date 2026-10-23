package controlplane_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/bishoku/vmcp/core/internal/controlplane"
	"github.com/bishoku/vmcp/core/internal/fixture"
	"github.com/bishoku/vmcp/core/internal/mcp"
	"github.com/bishoku/vmcp/core/internal/secrets"
	"github.com/bishoku/vmcp/core/internal/store"
	"github.com/bishoku/vmcp/core/internal/testdb"
)

const token = "admin-test-token"

type api struct {
	control *controlplane.Server
	t       *testing.T
	srv     *httptest.Server
	fixture *httptest.Server
	pool    *pgxpool.Pool
}

func setup(t *testing.T, configure ...func(*controlplane.Server)) *api {
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
	t.Cleanup(pool.Close)
	t.Cleanup(cancel)
	k := make([]byte, 32)
	_, _ = rand.Read(k)
	box, err := secrets.NewBox(base64.StdEncoding.EncodeToString(k))
	if err != nil {
		t.Fatal(err)
	}
	s := &controlplane.Server{
		Store:      &store.Store{Pool: pool, Box: box},
		AdminToken: token,
		Version:    "test",
		Log:        slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	t.Cleanup(s.CloseAudit)
	for _, apply := range configure {
		apply(s)
	}
	a := &api{control: s, pool: pool, t: t, srv: httptest.NewServer(s.Handler()), fixture: httptest.NewServer(fixture.New(fixture.Options{Token: "up-secret"}))}
	t.Cleanup(a.srv.Close)
	t.Cleanup(a.fixture.Close)
	return a
}

func (a *api) do(method, path string, body any, hdr map[string]string) (int, http.Header, map[string]any) {
	a.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, a.srv.URL+path, rd)
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		ct := "application/json"
		if method == http.MethodPatch {
			ct = "application/merge-patch+json"
		}
		req.Header.Set("Content-Type", ct)
	}
	for k, v := range hdr {
		if v == "" {
			req.Header.Del(k)
		} else {
			req.Header.Set(k, v)
		}
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		a.t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	if out == nil {
		out = map[string]any{"_raw": string(raw)}
	}
	return resp.StatusCode, resp.Header, out
}

func (a *api) expect(want int, method, path string, body any) (http.Header, map[string]any) {
	a.t.Helper()
	code, h, out := a.do(method, path, body, nil)
	if code != want {
		a.t.Fatalf("%s %s = %d, want %d: %v", method, path, code, want, out)
	}
	return h, out
}

func (a *api) upstream() map[string]any {
	return map[string]any{
		"slug": "fx", "name": "Fixture", "url": a.fixture.URL, "transport": "streamable_http",
		"headers": map[string]string{"Authorization": "Bearer up-secret", "X-Extra": "1"},
	}
}

func TestAuth(t *testing.T) {
	a := setup(t)
	for _, h := range []map[string]string{{"Authorization": ""}, {"Authorization": "Bearer wrong"}} {
		if code, _, _ := a.do("GET", "/api/v1/upstreams", nil, h); code != 401 {
			t.Fatalf("code=%d want 401", code)
		}
	}
	a.expect(200, "GET", "/api/v1/info", nil)
}

func TestUpstreamLifecycleAndSecrets(t *testing.T) {
	a := setup(t)
	_, out := a.expect(201, "POST", "/api/v1/upstreams", a.upstream())
	if _, leaked := out["headers"]; leaked {
		t.Fatal("headers returned")
	}
	if names, _ := out["headerNames"].([]any); len(names) != 2 {
		t.Fatalf("headerNames=%v", out["headerNames"])
	}
	a.expect(409, "POST", "/api/v1/upstreams", a.upstream())

	// PUT with identical content is idempotent (200), omitted headers keep the secret.
	put := a.upstream()
	delete(put, "headers")
	a.expect(200, "PUT", "/api/v1/upstreams/fx", put)

	// Merge-patch: null deletes a single header.
	h, out := a.expect(200, "PATCH", "/api/v1/upstreams/fx", map[string]any{"headers": map[string]any{"X-Extra": nil}})
	if names, _ := out["headerNames"].([]any); len(names) != 1 {
		t.Fatalf("headerNames=%v", names)
	}

	// Stale ETag -> 412.
	etag := h.Get("ETag")
	if etag == "" {
		t.Fatal("no etag")
	}
	code, _, _ := a.do("PATCH", "/api/v1/upstreams/fx", map[string]any{"name": "x"}, map[string]string{"If-Match": `"999"`})
	if code != 412 {
		t.Fatalf("code=%d want 412", code)
	}
	code, _, _ = a.do("PATCH", "/api/v1/upstreams/fx", map[string]any{"name": "x"}, map[string]string{"If-Match": etag})
	if code != 200 {
		t.Fatalf("code=%d want 200", code)
	}
	a.expect(404, "GET", "/api/v1/upstreams/nope", nil)
	a.expect(204, "DELETE", "/api/v1/upstreams/fx", nil)
	a.expect(404, "GET", "/api/v1/upstreams/fx", nil)
}

func TestRefreshVirtualServerAndDelete(t *testing.T) {
	a := setup(t)
	a.expect(201, "POST", "/api/v1/upstreams", a.upstream())
	_, out := a.expect(200, "POST", "/api/v1/upstreams/fx/refresh-tools", nil)
	if n, _ := out["toolCount"].(float64); n < 1 {
		t.Fatalf("toolCount=%v", out["toolCount"])
	}

	// Multi-error validation.
	code, _, out := a.do("POST", "/api/v1/virtual-servers", map[string]any{
		"slug": "vs", "name": "VS", "tools": []map[string]string{
			{"upstream": "ghost", "tool": "echo"}, {"upstream": "fx", "tool": "nope"},
		}}, nil)
	if code != 422 || !strings.Contains(toJSON(out), "unknown_upstream") || !strings.Contains(toJSON(out), "unknown_tool") {
		t.Fatalf("code=%d out=%v", code, out)
	}

	a.expect(201, "POST", "/api/v1/virtual-servers", map[string]any{
		"slug": "vs", "name": "VS", "tools": []map[string]string{{"upstream": "fx", "tool": "echo", "alias": "say"}}})
	_, out = a.expect(200, "GET", "/api/v1/virtual-servers/vs", nil)
	if !strings.Contains(toJSON(out), `"say"`) {
		t.Fatalf("alias missing: %v", out)
	}

	// Deleting an in-use upstream is refused unless forced.
	a.expect(409, "DELETE", "/api/v1/upstreams/fx", nil)
	a.expect(204, "DELETE", "/api/v1/upstreams/fx?force=true", nil)
}

func TestConnectionTest(t *testing.T) {
	a := setup(t)
	_, out := a.expect(200, "POST", "/api/v1/connection-tests", map[string]any{
		"url": a.fixture.URL, "transport": "streamable_http", "headers": map[string]string{"Authorization": "Bearer up-secret"}})
	if ok, _ := out["ok"].(bool); !ok {
		t.Fatalf("out=%v", out)
	}
	_, out = a.expect(200, "POST", "/api/v1/connection-tests", map[string]any{
		"url": a.fixture.URL, "transport": "streamable_http"})
	if ok, _ := out["ok"].(bool); ok {
		t.Fatalf("expected failure without token: %v", out)
	}
}

func TestExportApplyRoundTrip(t *testing.T) {
	a := setup(t)
	a.expect(201, "POST", "/api/v1/upstreams", a.upstream())
	a.expect(200, "POST", "/api/v1/upstreams/fx/refresh-tools", nil)
	a.expect(201, "POST", "/api/v1/virtual-servers", map[string]any{
		"slug": "vs", "name": "VS", "tools": []map[string]string{{"upstream": "fx", "tool": "echo"}}})
	_, doc := a.expect(200, "GET", "/api/v1/export", nil)

	// Dry-run of the same document changes nothing and doesn't error.
	a.expect(200, "POST", "/api/v1/apply?dryRun=true", doc)

	// Prune with a document lacking the virtual server removes it; secrets of kept upstream survive.
	doc["virtualServers"] = []any{}
	a.expect(200, "POST", "/api/v1/apply?prune=true", doc)
	a.expect(404, "GET", "/api/v1/virtual-servers/vs", nil)
	_, out := a.expect(200, "POST", "/api/v1/upstreams/fx/refresh-tools", nil)
	if n, _ := out["toolCount"].(float64); n < 1 {
		t.Fatalf("secret lost after apply: %v", out)
	}

	// Atomic: invalid document leaves state untouched.
	doc["virtualServers"] = []any{map[string]any{"slug": "bad", "name": "B",
		"tools": []any{map[string]any{"upstream": "ghost", "tool": "x"}}}}
	if code, _, _ := a.do("POST", "/api/v1/apply", doc, nil); code != 422 {
		t.Fatalf("code=%d want 422", code)
	}
	a.expect(404, "GET", "/api/v1/virtual-servers/bad", nil)
}

func TestPagination(t *testing.T) {
	a := setup(t)
	for _, s := range []string{"a", "b", "c"} {
		a.expect(201, "POST", "/api/v1/upstreams", map[string]any{"slug": s, "name": s, "url": a.fixture.URL, "transport": "streamable_http"})
	}
	_, out := a.expect(200, "GET", "/api/v1/upstreams?limit=2", nil)
	items, _ := out["items"].([]any)
	if len(items) != 2 || out["nextCursor"] == nil {
		t.Fatalf("page1=%v", out)
	}
	_, out = a.expect(200, "GET", "/api/v1/upstreams?limit=2&cursor="+out["nextCursor"].(string), nil)
	if items, _ := out["items"].([]any); len(items) != 1 {
		t.Fatalf("page2=%v", out)
	}
}

func toJSON(v any) string { b, _ := json.Marshal(v); return string(b) }

func TestLegacySSEUpstream(t *testing.T) {
	a := setup(t)
	legacy := httptest.NewServer(fixture.NewLegacySSE(fixture.Options{}))
	defer legacy.Close()
	a.expect(201, "POST", "/api/v1/upstreams", map[string]any{"slug": "old", "name": "Old", "url": legacy.URL, "transport": "sse"})
	_, out := a.expect(200, "POST", "/api/v1/upstreams/old/refresh-tools", nil)
	if n, _ := out["toolCount"].(float64); n < 1 {
		t.Fatalf("out=%v", out)
	}
}

func TestAgentKeys(t *testing.T) {
	a := setup(t)
	a.expect(201, "POST", "/api/v1/virtual-servers", map[string]any{"slug": "vs", "name": "VS"})

	_, out := a.expect(201, "POST", "/api/v1/agent-keys", map[string]any{
		"slug": "bot", "description": "ci bot", "virtualServers": []string{"vs"}, "ipAllowlist": []string{"10.0.0.0/8", "192.168.1.7"},
		"expiresAt": "2099-01-01T00:00:00Z"})
	secret, _ := out["key"].(string)
	if !strings.HasPrefix(secret, "vmcp_") || out["keyPrefix"] != secret[:12] {
		t.Fatalf("secret/prefix wrong: %v", out)
	}
	if ips, _ := out["ipAllowlist"].([]any); len(ips) != 2 || ips[1] != "192.168.1.7/32" {
		t.Fatalf("ips not normalized: %v", out["ipAllowlist"])
	}

	// The secret is never shown again.
	_, out = a.expect(200, "GET", "/api/v1/agent-keys/bot", nil)
	if _, leaked := out["key"]; leaked || out["status"] != "active" {
		t.Fatalf("out=%v", out)
	}

	// Validation: unknown server and bad IP, all reported.
	code, _, out := a.do("POST", "/api/v1/agent-keys", map[string]any{"slug": "bad", "virtualServers": []string{"ghost"}, "ipAllowlist": []string{"nope"}}, nil)
	if code != 422 || !strings.Contains(toJSON(out), "unknown_virtual_server") || !strings.Contains(toJSON(out), "/ipAllowlist/0") {
		t.Fatalf("code=%d out=%v", code, out)
	}
	a.expect(409, "POST", "/api/v1/agent-keys", map[string]any{"slug": "bot"})

	// Merge-patch: explicit null clears expiry; absent fields stay.
	_, out = a.expect(200, "PATCH", "/api/v1/agent-keys/bot", map[string]any{"expiresAt": nil})
	if out["expiresAt"] != nil || out["description"] != "ci bot" {
		t.Fatalf("out=%v", out)
	}
	a.expect(422, "PATCH", "/api/v1/agent-keys/bot", map[string]any{"expiresAt": "2000-01-01T00:00:00Z"})

	// Rotation replaces the secret.
	_, out = a.expect(200, "POST", "/api/v1/agent-keys/bot/rotate", nil)
	if out["key"] == secret || out["key"] == nil {
		t.Fatalf("rotate did not issue a new key: %v", out)
	}

	_, out = a.expect(200, "POST", "/api/v1/agent-keys/bot/revoke", nil)
	if out["status"] != "revoked" {
		t.Fatalf("out=%v", out)
	}
	a.expect(204, "DELETE", "/api/v1/agent-keys/bot", nil)
	a.expect(404, "GET", "/api/v1/agent-keys/bot", nil)

	// Authorization is required and virtual servers default to authRequired=true.
	_, out = a.expect(200, "GET", "/api/v1/virtual-servers/vs", nil)
	if out["authRequired"] != true {
		t.Fatalf("authRequired default: %v", out)
	}
}

func TestAuditEventsAPI(t *testing.T) {
	a := setup(t)
	for i, st := range []string{"ok", "ok", "denied", "tool_error"} {
		if _, err := a.pool.Exec(context.Background(),
			`INSERT INTO audit_events (virtual_server, agent_key, method, tool, status, duration_ms) VALUES ('vs', $1, 'tools/call', $2, $3, $4)`,
			"bot", "t"+string(rune('a'+i)), st, i); err != nil {
			t.Fatal(err)
		}
	}
	_, out := a.expect(200, "GET", "/api/v1/audit-events?limit=3", nil)
	items, _ := out["items"].([]any)
	if len(items) != 3 || out["nextCursor"] == nil {
		t.Fatalf("page1=%v", out)
	}
	if first := items[0].(map[string]any); first["tool"] != "td" { // newest first
		t.Fatalf("order: %v", first)
	}
	_, out = a.expect(200, "GET", "/api/v1/audit-events?limit=3&cursor="+out["nextCursor"].(string), nil)
	if items, _ := out["items"].([]any); len(items) != 1 || out["nextCursor"] != nil {
		t.Fatalf("page2=%v", out)
	}
	_, out = a.expect(200, "GET", "/api/v1/audit-events?status=denied&agentKey=bot", nil)
	if items, _ := out["items"].([]any); len(items) != 1 {
		t.Fatalf("filter=%v", out)
	}
	a.expect(422, "GET", "/api/v1/audit-events?status=bogus&from=yesterday", nil)
}

func TestVirtualServerPII(t *testing.T) {
	a := setup(t)
	body := map[string]any{"name": "VS", "pii": map[string]any{
		"enabled": true, "detectors": []string{"email", "iban"}, "maskArguments": true,
		"customRules": []map[string]string{{"name": "emp", "pattern": "EMP-[0-9]{6}"}}}}
	a.expect(201, "PUT", "/api/v1/virtual-servers/vs", body)
	_, out := a.expect(200, "GET", "/api/v1/virtual-servers/vs", nil)
	p, _ := out["pii"].(map[string]any)
	if p["enabled"] != true || p["maskArguments"] != true || len(p["customRules"].([]any)) != 1 {
		t.Fatalf("pii not persisted: %v", out["pii"])
	}
	a.expect(200, "PUT", "/api/v1/virtual-servers/vs", body) // identical -> unchanged, no revision bump needed
	_, out = a.expect(200, "PATCH", "/api/v1/virtual-servers/vs", map[string]any{"pii": map[string]any{"enabled": false}})
	if out["pii"].(map[string]any)["enabled"] != false {
		t.Fatalf("patch: %v", out)
	}

	code, _, out := a.do("PUT", "/api/v1/virtual-servers/vs", map[string]any{"name": "VS", "pii": map[string]any{
		"enabled": true, "detectors": []string{"bogus"}, "customRules": []map[string]string{{"name": "x", "pattern": "("}}}}, nil)
	s := toJSON(out)
	if code != 422 || !strings.Contains(s, "/pii/detectors/0") || !strings.Contains(s, "/pii/customRules/0/pattern") {
		t.Fatalf("code=%d %s", code, s)
	}

	// Export carries the policy and re-applying it is stable.
	_, doc := a.expect(200, "GET", "/api/v1/export", nil)
	a.expect(200, "POST", "/api/v1/apply?dryRun=true", doc)
}

func (a *api) withToken(tok, method, path string, body any) (int, map[string]any) {
	a.t.Helper()
	code, _, out := a.do(method, path, body, map[string]string{"Authorization": "Bearer " + tok})
	return code, out
}

func TestAdminTokens(t *testing.T) {
	a := setup(t)
	_, out := a.expect(201, "POST", "/api/v1/admin-tokens", map[string]any{"slug": "ci", "scope": "read", "description": "ci reader"})
	secret, _ := out["token"].(string)
	if !strings.HasPrefix(secret, "vmcpa_") || out["tokenPrefix"] != secret[:12] || out["scope"] != "read" {
		t.Fatalf("out=%v", out)
	}
	if _, leaked := a.getJSON("/api/v1/admin-tokens/ci")["token"]; leaked {
		t.Fatal("secret returned on read")
	}

	// A read token can read but not write, and cannot manage tokens.
	if code, _ := a.withToken(secret, "GET", "/api/v1/upstreams", nil); code != 200 {
		t.Fatalf("read token GET: %d", code)
	}
	for _, c := range []struct{ m, p string }{{"POST", "/api/v1/upstreams"}, {"DELETE", "/api/v1/upstreams/x"}, {"POST", "/api/v1/admin-tokens"}} {
		if code, out := a.withToken(secret, c.m, c.p, map[string]any{}); code != 403 || out["code"] != "forbidden" {
			t.Fatalf("%s %s: %d %v", c.m, c.p, code, out)
		}
	}

	// Promote to admin: now it can write.
	a.expect(200, "PATCH", "/api/v1/admin-tokens/ci", map[string]any{"scope": "admin"})
	if code, _ := a.withToken(secret, "POST", "/api/v1/upstreams", map[string]any{"slug": "u1", "name": "U", "url": a.fixture.URL, "transport": "streamable_http"}); code != 201 {
		t.Fatalf("admin token POST: %d", code)
	}

	// Rotation kills the old secret; revocation kills the new one; unknown prefix-less tokens never work.
	_, out = a.expect(200, "POST", "/api/v1/admin-tokens/ci/rotate", nil)
	newSecret := out["token"].(string)
	if code, _ := a.withToken(secret, "GET", "/api/v1/info", nil); code != 401 {
		t.Fatalf("old secret after rotate: %d", code)
	}
	if code, _ := a.withToken(newSecret, "GET", "/api/v1/info", nil); code != 200 {
		t.Fatalf("new secret: %d", code)
	}
	a.expect(200, "POST", "/api/v1/admin-tokens/ci/revoke", nil)
	if code, _ := a.withToken(newSecret, "GET", "/api/v1/info", nil); code != 401 {
		t.Fatalf("revoked: %d", code)
	}
	if code, _ := a.withToken("vmcpa_bogus", "GET", "/api/v1/info", nil); code != 401 {
		t.Fatalf("bogus: %d", code)
	}

	// Validation and expiry.
	a.expect(422, "POST", "/api/v1/admin-tokens", map[string]any{"slug": "bad", "scope": "root"})
	a.expect(201, "POST", "/api/v1/admin-tokens", map[string]any{"slug": "tmp", "scope": "admin", "expiresAt": "2099-01-01T00:00:00Z"})
	a.expect(409, "POST", "/api/v1/admin-tokens", map[string]any{"slug": "tmp"})
	a.expect(204, "DELETE", "/api/v1/admin-tokens/tmp", nil)
	a.expect(404, "GET", "/api/v1/admin-tokens/tmp", nil)
}

func (a *api) getJSON(path string) map[string]any {
	a.t.Helper()
	_, out := a.expect(200, "GET", path, nil)
	return out
}

func TestUpstreamFallbackURLs(t *testing.T) {
	a := setup(t)
	_, out := a.expect(201, "POST", "/api/v1/upstreams", map[string]any{
		"slug": "ha", "name": "HA", "url": "http://a.example/mcp", "fallbackUrls": []string{"http://b.example/mcp"},
	})
	if fb, _ := out["fallbackUrls"].([]any); len(fb) != 1 {
		t.Fatalf("fallbackUrls not stored: %v", out)
	}
	if h, _ := out["health"].(map[string]any); h["state"] != "unknown" {
		t.Fatalf("an upstream nobody probed has unknown health: %v", out["health"])
	}
	code, _, body := a.do("PUT", "/api/v1/upstreams/ha", map[string]any{
		"name": "HA", "url": "http://a.example/mcp", "fallbackUrls": []string{"ftp://x", "http://a.example/mcp"},
	}, nil)
	s := fmt.Sprint(body)
	if code != 422 || !strings.Contains(s, "/fallbackUrls/0") || !strings.Contains(s, "/fallbackUrls/1") {
		t.Fatalf("want both fallback URLs rejected, got %d %s", code, s)
	}
	_, out = a.expect(200, "PATCH", "/api/v1/upstreams/ha", map[string]any{"fallbackUrls": []string{}})
	if fb, _ := out["fallbackUrls"].([]any); len(fb) != 0 {
		t.Fatalf("PATCH must clear fallbacks: %v", out)
	}
}

func TestRateLimitFields(t *testing.T) {
	a := setup(t)
	_, out := a.expect(201, "POST", "/api/v1/agent-keys", map[string]any{"slug": "k1", "rateLimitPerMinute": 60})
	if out["rateLimitPerMinute"] != float64(60) {
		t.Fatalf("key limit not stored: %v", out)
	}
	code, _, body := a.do("POST", "/api/v1/agent-keys", map[string]any{"slug": "k2", "rateLimitPerMinute": 0}, nil)
	if code != 422 || !strings.Contains(fmt.Sprint(body), "/rateLimitPerMinute") {
		t.Fatalf("limit 0 must be rejected: %d %v", code, body)
	}
	_, out = a.expect(200, "PATCH", "/api/v1/agent-keys/k1", map[string]any{"description": "x"})
	if out["rateLimitPerMinute"] != float64(60) {
		t.Fatalf("an unrelated PATCH must keep the limit: %v", out)
	}
	_, out = a.expect(200, "PATCH", "/api/v1/agent-keys/k1", map[string]any{"rateLimitPerMinute": nil})
	if out["rateLimitPerMinute"] != nil {
		t.Fatalf("null must clear the limit: %v", out)
	}

	_, out = a.expect(201, "POST", "/api/v1/virtual-servers", map[string]any{"slug": "vs", "name": "VS", "rateLimitPerMinute": 120})
	if out["rateLimitPerMinute"] != float64(120) {
		t.Fatalf("virtual server limit not stored: %v", out)
	}
	_, out = a.expect(200, "PATCH", "/api/v1/virtual-servers/vs", map[string]any{"name": "VS2"})
	if out["rateLimitPerMinute"] != float64(120) {
		t.Fatalf("PATCH without the field must keep the limit: %v", out)
	}
	_, out = a.expect(200, "PATCH", "/api/v1/virtual-servers/vs", map[string]any{"rateLimitPerMinute": nil})
	if out["rateLimitPerMinute"] != nil {
		t.Fatalf("null must clear the limit: %v", out)
	}
}

func TestLongCapabilityDescriptionOverrides(t *testing.T) {
	a := setup(t)
	a.expect(201, "POST", "/api/v1/upstreams", a.upstream())
	a.expect(200, "POST", "/api/v1/upstreams/fx/refresh-tools", nil)
	database := &store.Store{Pool: a.pool}
	if err := database.Do(context.Background(), func(tx *store.Tx) error {
		return tx.ReplaceCatalog(context.Background(), "fx", mcp.Catalog{
			Resources: []mcp.CatalogObject{mcp.CatalogObject{}.With("uri", "doc://guide").With("name", "Guide").With("description", "Resource source")},
			Templates: []mcp.CatalogObject{mcp.CatalogObject{}.With("uriTemplate", "doc://{id}").With("name", "Template").With("description", "Template source")},
			Prompts:   []mcp.CatalogObject{mcp.CatalogObject{}.With("name", "review").With("description", "Prompt source")},
		})
	}); err != nil {
		t.Fatal(err)
	}
	// Exactly 20,000 code points; UTF-8 bytes and UTF-16 units exceed that count.
	text := strings.Repeat("ş🙂", 9999) + "\n "
	tools := []map[string]string{{"upstream": "fx", "tool": "echo", "alias": "say", "description": text}}
	catalog := []map[string]string{
		{"upstream": "fx", "kind": "resource", "name": "doc://guide", "description": text},
		{"upstream": "fx", "kind": "template", "name": "doc://{id}", "description": text},
		{"upstream": "fx", "kind": "prompt", "name": "review", "description": text},
	}
	a.expect(201, "POST", "/api/v1/virtual-servers", map[string]any{"slug": "long", "name": "Long descriptions", "tools": tools, "catalog": catalog})
	assertDescriptions := func(want string) {
		t.Helper()
		_, out := a.expect(200, "GET", "/api/v1/virtual-servers/long", nil)
		for _, family := range []string{"tools", "catalog"} {
			for _, raw := range out[family].([]any) {
				actual, _ := raw.(map[string]any)["description"].(string)
				if actual != want {
					t.Fatalf("%s description changed during round trip", family)
				}
			}
		}
	}
	assertDescriptions(text)
	tools[0]["description"] = text + "x"
	_, out := a.expect(422, "PATCH", "/api/v1/virtual-servers/long", map[string]any{"tools": tools})
	if !strings.Contains(toJSON(out), "/tools/0/description") {
		t.Fatalf("missing tool field error: %v", out)
	}
	for i := range catalog {
		catalog[i]["description"] = text + "x"
		_, out = a.expect(422, "PATCH", "/api/v1/virtual-servers/long", map[string]any{"catalog": catalog})
		if !strings.Contains(toJSON(out), fmt.Sprintf("/catalog/%d/description", i)) {
			t.Fatalf("missing catalog field error: %v", out)
		}
		catalog[i]["description"] = text
	}
	// Rejected updates leave every saved selection intact.
	assertDescriptions(text)
	tools[0]["description"] = ""
	for i := range catalog {
		catalog[i]["description"] = ""
	}
	a.expect(200, "PATCH", "/api/v1/virtual-servers/long", map[string]any{"tools": tools, "catalog": catalog})
	assertDescriptions("")
}
