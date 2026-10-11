package e2e

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bishoku/vmcp/core/internal/agentauth"
	"github.com/bishoku/vmcp/core/internal/fixture"
	"github.com/bishoku/vmcp/core/internal/snapshot"
	"github.com/bishoku/vmcp/core/internal/store"
	"github.com/bishoku/vmcp/core/internal/testidp"
	"github.com/bishoku/vmcp/core/internal/virtual"
)

const consistencyInit = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"consistency","version":"1"}}}`
const consistencyPing = `{"jsonrpc":"2.0","id":2,"method":"ping"}`
const consistencyCall = `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"alpha__whoami","arguments":{}}}`

func consistencyRequest(t *testing.T, url, method, sid, token, body string) (*http.Response, string) {
	t.Helper()
	req, _ := http.NewRequest(method, url, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if sid != "" {
		req.Header.Set("Mcp-Session-Id", sid)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp, string(b)
}

func consistencyNode(t *testing.T, e *env, jwt *agentauth.Verifier) (string, *snapshot.Syncer, *virtual.Manager) {
	t.Helper()
	m := virtual.NewManager()
	m.Store = virtual.PGSessionStore{Pool: e.pool}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	s := &snapshot.Syncer{Pool: e.pool, Box: e.syncer.Box, Log: log, PollInterval: 20 * time.Millisecond, MaxAuthorizationAge: 2 * time.Second, OnChange: m.Reconcile}
	if err := s.Start(e.ctx); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	(&virtual.Handler{Snap: s, Sessions: m, Log: log, JWT: jwt}).Register(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL + "/mcp/v1/virtual/agent", s, m
}

func awaitConsistency(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatal("configuration did not converge")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestTwoWarmNodesRevokeGrantsRotateCredentialsAndTerminate(t *testing.T) {
	e := setup(t)
	token := "vmcp_consistency_secret"
	e.exec(t, `UPDATE virtual_servers SET auth_required=true WHERE slug='agent'`)
	e.exec(t, `INSERT INTO agent_keys(tenant_id,slug,key_hash,key_prefix) SELECT id,'consistency',$1,'vmcp_cons' FROM tenants WHERE slug='default'`, store.HashKey(token))
	e.exec(t, `INSERT INTO agent_key_servers SELECT k.id,v.id FROM agent_keys k,virtual_servers v WHERE k.slug='consistency' AND v.slug='agent'`)
	a, sa, ma := consistencyNode(t, e, nil)
	b, sb, mb := consistencyNode(t, e, nil)
	response, body := consistencyRequest(t, a, "POST", "", token, consistencyInit)
	sid := response.Header.Get("Mcp-Session-Id")
	if sid == "" {
		t.Fatalf("initialize: %d %s", response.StatusCode, body)
	}
	if resp, _ := consistencyRequest(t, a, "POST", sid, token, `{"jsonrpc":"2.0","method":"notifications/initialized"}`); resp.StatusCode != 202 {
		t.Fatal(resp.Status)
	}
	checkCalls := func(expected string) {
		t.Helper()
		for _, url := range []string{a, b} {
			resp, body := consistencyRequest(t, url, "POST", sid, token, consistencyCall)
			if resp.StatusCode != 200 || !strings.Contains(body, expected) {
				t.Fatalf("call %s: %d %s", url, resp.StatusCode, body)
			}
		}
	}
	checkCalls("Bearer fixture-secret")
	if ma.Len() != 1 || mb.Len() != 1 {
		t.Fatal("both node caches must be warm")
	}
	syncRevision := func() {
		t.Helper()
		rev, err := store.Revision(e.ctx, e.pool)
		if err != nil {
			t.Fatal(err)
		}
		awaitConsistency(t, func() bool { return sa.Snapshot().Revision >= rev && sb.Snapshot().Revision >= rev })
	}
	// The same session must use both the new URL and the new header on each node.
	rotated := httptest.NewServer(fixture.New(fixture.Options{Token: "rotated-secret"}))
	defer rotated.Close()
	enc, err := e.syncer.Box.SealHeadersBound(map[string]string{"Authorization": "Bearer rotated-secret"}, "00000000-0000-4000-8000-000000000001")
	if err != nil {
		t.Fatal(err)
	}
	e.exec(t, `UPDATE upstreams SET url=$1,encrypted_headers=$2 WHERE slug='alpha'`, rotated.URL, enc)
	syncRevision()
	checkCalls("Bearer rotated-secret")
	// A grant change is reevaluated without evicting either session.
	e.exec(t, `DELETE FROM agent_key_servers WHERE agent_key_id=(SELECT id FROM agent_keys WHERE slug='consistency')`)
	syncRevision()
	for _, url := range []string{a, b} {
		resp, _ := consistencyRequest(t, url, "POST", sid, token, consistencyPing)
		if resp.StatusCode != 403 {
			t.Fatalf("revoked grant: %d", resp.StatusCode)
		}
	}
	e.exec(t, `INSERT INTO agent_key_servers SELECT k.id,v.id FROM agent_keys k,virtual_servers v WHERE k.slug='consistency' AND v.slug='agent'`)
	syncRevision()
	checkCalls("Bearer rotated-secret")
	// Revocation remains effective with initialized sessions and pooled upstream clients.
	e.exec(t, `UPDATE agent_keys SET revoked_at=now() WHERE slug='consistency'`)
	syncRevision()
	for _, url := range []string{a, b} {
		resp, _ := consistencyRequest(t, url, "POST", sid, token, consistencyCall)
		if resp.StatusCode != 401 {
			t.Fatalf("revoked key: %d", resp.StatusCode)
		}
	}
	e.exec(t, `UPDATE agent_keys SET revoked_at=NULL WHERE slug='consistency'`)
	syncRevision()
	checkCalls("Bearer rotated-secret")
	if resp, _ := consistencyRequest(t, a, "DELETE", sid, token, ""); resp.StatusCode != 204 {
		t.Fatalf("DELETE: %d", resp.StatusCode)
	}
	if resp, _ := consistencyRequest(t, b, "POST", sid, token, consistencyCall); resp.StatusCode != 404 {
		t.Fatalf("warm node reused terminated session: %d", resp.StatusCode)
	}
	var terminated bool
	if err := e.pool.QueryRow(e.ctx, `SELECT terminated_at IS NOT NULL FROM sessions WHERE id=$1`, sid).Scan(&terminated); err != nil || !terminated {
		t.Fatalf("tombstone: %v %v", terminated, err)
	}
}

func TestOAuthSessionOwnershipAcrossNodes(t *testing.T) {
	e := setup(t)
	e.exec(t, `UPDATE virtual_servers SET auth_required=true WHERE slug='agent'`)
	e.exec(t, `INSERT INTO identity_bindings(tenant_id,slug,conditions) SELECT id,'shared','[{"claim":"azp","op":"equals","value":"shared-app"}]'::jsonb FROM tenants WHERE slug='default'`)
	e.exec(t, `INSERT INTO identity_binding_servers SELECT b.id,v.id FROM identity_bindings b,virtual_servers v WHERE b.slug='shared' AND v.slug='agent'`)
	idp := testidp.New(t)
	verifier, err := agentauth.NewVerifier(e.ctx, agentauth.Config{Issuer: idp.URL(), DiscoveryURL: idp.URL(), Audience: oauthAud})
	if err != nil {
		t.Fatal(err)
	}
	token := func(subject string) string {
		return idp.Mint(t, testidp.Token{Subject: subject, Audience: []string{oauthAud}, Claims: map[string]any{"azp": "shared-app"}})
	}
	alice, bob := token("alice"), token("bob")
	a, _, _ := consistencyNode(t, e, verifier)
	b, _, mb := consistencyNode(t, e, verifier)
	resp, body := consistencyRequest(t, a, "POST", "", alice, consistencyInit)
	sid := resp.Header.Get("Mcp-Session-Id")
	if sid == "" {
		t.Fatalf("initialize: %d %s", resp.StatusCode, body)
	}
	if resp, _ := consistencyRequest(t, b, "POST", sid, alice, consistencyPing); resp.StatusCode != 200 {
		t.Fatalf("same owner resume: %d", resp.StatusCode)
	}
	if mb.Len() != 1 {
		t.Fatal("cache not warm")
	}
	for _, url := range []string{a, b} {
		for _, method := range []string{"POST", "GET", "DELETE"} {
			resp, _ := consistencyRequest(t, url, method, sid, bob, consistencyPing)
			if resp.StatusCode != 404 {
				t.Fatalf("cross-subject %s: %d", method, resp.StatusCode)
			}
		}
	}
	// Renewing a token for the same verified identity preserves ownership.
	if resp, _ := consistencyRequest(t, b, "POST", sid, token("alice"), consistencyPing); resp.StatusCode != 200 {
		t.Fatalf("token refresh: %d", resp.StatusCode)
	}
	var mode, credential, issuer, subject string
	if err := e.pool.QueryRow(e.ctx, `SELECT owner_mode,credential_id,owner_issuer,owner_subject FROM sessions WHERE id=$1`, sid).Scan(&mode, &credential, &issuer, &subject); err != nil {
		t.Fatal(err)
	}
	if mode != "oauth" || credential == "" || issuer != idp.URL() || subject != "alice" {
		t.Fatalf("incomplete persisted owner: %s %s %s %s", mode, credential, issuer, subject)
	}
	// A differently configured issuer must not adopt an existing session, even for the same subject.
	other := testidp.New(t)
	otherVerifier, err := agentauth.NewVerifier(e.ctx, agentauth.Config{Issuer: other.URL(), DiscoveryURL: other.URL(), Audience: oauthAud})
	if err != nil {
		t.Fatal(err)
	}
	c, _, _ := consistencyNode(t, e, otherVerifier)
	otherToken := other.Mint(t, testidp.Token{Subject: "alice", Audience: []string{oauthAud}, Claims: map[string]any{"azp": "shared-app"}})
	if resp, _ := consistencyRequest(t, c, "POST", sid, otherToken, consistencyPing); resp.StatusCode != 404 {
		t.Fatalf("cross-issuer adoption: %d", resp.StatusCode)
	}
	// Recreating a binding with the same slug does not recreate its credential identity.
	e.exec(t, `DELETE FROM identity_bindings WHERE slug='shared'`)
	e.exec(t, `INSERT INTO identity_bindings(tenant_id,slug,conditions) SELECT id,'shared','[{"claim":"azp","op":"equals","value":"shared-app"}]'::jsonb FROM tenants WHERE slug='default'`)
	e.exec(t, `INSERT INTO identity_binding_servers SELECT b.id,v.id FROM identity_bindings b,virtual_servers v WHERE b.slug='shared' AND v.slug='agent'`)
	var replacement string
	if err := e.pool.QueryRow(e.ctx, `SELECT id::text FROM identity_bindings WHERE slug='shared'`).Scan(&replacement); err != nil {
		t.Fatal(err)
	}
	// Confirm the new ID is in each provider before exercising the old session.
	awaitConsistency(t, func() bool {
		resp, _ := consistencyRequest(t, b, "POST", sid, alice, consistencyPing)
		return resp.StatusCode == 404
	})
	if replacement == credential {
		t.Fatal("binding ID was not replaced")
	}
}
