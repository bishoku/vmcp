package e2e

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bishoku/vmcp/core/internal/agentauth"
	"github.com/bishoku/vmcp/core/internal/audit"
	"github.com/bishoku/vmcp/core/internal/ratelimit"
	"github.com/bishoku/vmcp/core/internal/testidp"
	"github.com/bishoku/vmcp/core/internal/upstream"
	"github.com/bishoku/vmcp/core/internal/virtual"
)

const oauthAud = "vmcp-gateway"

func TestOAuthBindings(t *testing.T) {
	e := setup(t)
	idp := testidp.New(t)
	ver, err := agentauth.NewVerifier(e.ctx, agentauth.Config{Issuer: idp.URL(), DiscoveryURL: idp.URL(), Audience: oauthAud})
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	aw := audit.NewWriter(e.ctx, e.pool, log)
	mux := http.NewServeMux()
	(&virtual.Handler{Snap: e.syncer, Sessions: virtual.NewManager(), Log: log, JWT: ver, Limiter: ratelimit.NewLocal(),
		Audit: aw, PublicURL: "https://gw.example.com"}).Register(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	url := srv.URL + "/mcp/v1/virtual/agent"

	e.exec(t, `UPDATE virtual_servers SET auth_required = true WHERE slug = 'agent'`)
	token := func(claims map[string]any) string {
		return idp.Mint(t, testidp.Token{Subject: "svc-1", Audience: []string{oauthAud}, Claims: claims})
	}
	post := func(tok, body string) (*http.Response, string) {
		t.Helper()
		req, _ := http.NewRequest("POST", url, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		if tok != "" {
			req.Header.Set("Authorization", "Bearer "+tok)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp, string(b)
	}
	const initBody = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`
	good := token(map[string]any{"azp": "billing-bot"})

	// Wait for auth_required to reach the gateway.
	for end := time.Now().Add(5 * time.Second); time.Now().Before(end); time.Sleep(50 * time.Millisecond) {
		if r, _ := post("", initBody); r.StatusCode == 401 {
			break
		}
	}

	// Deny by default: valid token but no binding.
	if r, _ := post(good, initBody); r.StatusCode != 403 {
		t.Fatalf("no binding: want 403, got %d", r.StatusCode)
	}

	// Bad token: 401 with the discovery challenge.
	r, _ := post("not.a.jwt", initBody)
	if r.StatusCode != 401 {
		t.Fatalf("bad token: %d", r.StatusCode)
	}
	r, _ = post(idp.Mint(t, testidp.Token{Subject: "x", Audience: []string{"someone-else"}}), initBody)
	ch := r.Header.Get("WWW-Authenticate")
	if r.StatusCode != 401 || !strings.Contains(ch, `error="invalid_token"`) ||
		!strings.Contains(ch, `resource_metadata="https://gw.example.com/.well-known/oauth-protected-resource/mcp/v1/virtual/agent"`) {
		t.Fatalf("wrong-audience: %d %q", r.StatusCode, ch)
	}
	if r, _ := post("", initBody); r.StatusCode != 401 || strings.Contains(r.Header.Get("WWW-Authenticate"), "invalid_token") {
		t.Fatalf("missing credentials: %d %q", r.StatusCode, r.Header.Get("WWW-Authenticate"))
	}

	// Protected-resource metadata.
	resp, err := http.Get(srv.URL + "/.well-known/oauth-protected-resource/mcp/v1/virtual/agent")
	if err != nil {
		t.Fatal(err)
	}
	var meta struct {
		Resource             string   `json:"resource"`
		AuthorizationServers []string `json:"authorization_servers"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&meta)
	resp.Body.Close()
	if resp.StatusCode != 200 || meta.Resource != "https://gw.example.com/mcp/v1/virtual/agent" || len(meta.AuthorizationServers) != 1 {
		t.Fatalf("metadata: %d %+v", resp.StatusCode, meta)
	}
	if resp, _ := http.Get(srv.URL + "/.well-known/oauth-protected-resource/mcp/v1/virtual/nope"); resp.StatusCode != 404 {
		t.Fatalf("metadata for unknown server: %d", resp.StatusCode)
	}

	// Create a binding (DB triggers push it to the gateway).
	e.exec(t, `INSERT INTO identity_bindings (tenant_id, slug, conditions, rate_limit_per_minute)
		SELECT id, 'billing', '[{"claim":"azp","op":"equals","value":"billing-bot"}]'::jsonb, 3 FROM tenants WHERE slug='default'`)
	e.exec(t, `INSERT INTO identity_binding_servers (binding_id, virtual_server_id)
		SELECT b.id, v.id FROM identity_bindings b, virtual_servers v WHERE b.slug='billing' AND v.slug='agent'`)

	var sid string
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		r, _ := post(good, initBody)
		if r.StatusCode == 200 {
			sid = r.Header.Get("Mcp-Session-Id")
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if sid == "" {
		t.Fatal("binding never took effect")
	}

	// Complete the manually opened session before using it for rate-limit checks.
	initialized, _ := http.NewRequest("POST", url, strings.NewReader(`{"jsonrpc":"2.0","method":"notifications/initialized"}`))
	initialized.Header.Set("Content-Type", "application/json")
	initialized.Header.Set("Accept", "application/json, text/event-stream")
	initialized.Header.Set("Authorization", "Bearer "+good)
	initialized.Header.Set("Mcp-Session-Id", sid)
	initializedResp, err := http.DefaultClient.Do(initialized)
	if err != nil {
		t.Fatal(err)
	}
	initializedResp.Body.Close()
	if initializedResp.StatusCode != http.StatusAccepted {
		t.Fatalf("initialized: %s", initializedResp.Status)
	}

	// Real MCP client with the token.
	c := upstream.NewClient(url, map[string]string{"Authorization": "Bearer " + good}, nil)
	if _, err := c.Initialize(e.ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close(context.Background()) })
	if got := call(t, c, "alpha__add", `{"a":2,"b":3}`); got.IsError || got.Content[0].Text != "5" {
		t.Fatalf("call failed: %+v", got)
	}

	// A token the binding does not match stays denied.
	if r, _ := post(token(map[string]any{"azp": "other"}), initBody); r.StatusCode != 403 {
		t.Fatalf("non-matching claims: %d", r.StatusCode)
	}

	// Per-binding rate limit keyed oauth:<slug>.
	callBody := `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"alpha__add","arguments":{"a":1,"b":1}}}`
	limited := false
	for i := 0; i < 20 && !limited; i++ {
		rr, _ := post(good, callBody)
		_ = rr
		req, _ := http.NewRequest("POST", url, strings.NewReader(callBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		req.Header.Set("Authorization", "Bearer "+good)
		req.Header.Set("Mcp-Session-Id", sid)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		limited = resp.StatusCode == http.StatusTooManyRequests
	}
	if !limited {
		t.Fatal("binding rate limit never applied")
	}

	// Audit carries the principal and the subject.
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var n int
		_ = e.pool.QueryRow(e.ctx, `SELECT count(*) FROM audit_events WHERE agent_key = 'oauth:billing' AND subject = 'svc-1'`).Scan(&n)
		if n > 0 {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("no audit event with oauth principal and subject")
}

func TestOAuthIPAllowlist(t *testing.T) {
	e := setup(t)
	idp := testidp.New(t)
	ver, err := agentauth.NewVerifier(e.ctx, agentauth.Config{Issuer: idp.URL(), DiscoveryURL: idp.URL(), Audience: oauthAud})
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	mux := http.NewServeMux()
	(&virtual.Handler{Snap: e.syncer, Sessions: virtual.NewManager(), Log: log, JWT: ver}).Register(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	e.exec(t, `UPDATE virtual_servers SET auth_required = true WHERE slug = 'agent'`)
	e.exec(t, `INSERT INTO identity_bindings (tenant_id, slug, conditions, ip_allowlist)
		SELECT id, 'locked', '[{"claim":"sub","op":"exists"}]'::jsonb, ARRAY['10.9.9.0/24'] FROM tenants WHERE slug='default'`)
	e.exec(t, `INSERT INTO identity_binding_servers (binding_id, virtual_server_id)
		SELECT b.id, v.id FROM identity_bindings b, virtual_servers v WHERE b.slug='locked' AND v.slug='agent'`)
	tok := idp.Mint(t, testidp.Token{Subject: "s", Audience: []string{oauthAud}})
	time.Sleep(500 * time.Millisecond) // let the NOTIFY land
	req, _ := http.NewRequest("POST", srv.URL+"/mcp/v1/virtual/agent", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatalf("client outside allowlist: want 403, got %d", resp.StatusCode)
	}
}
