package controlplane_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/bishoku/vmcp/core/internal/controlplane"
	"github.com/bishoku/vmcp/core/internal/oidc"
	"github.com/bishoku/vmcp/core/internal/store"
	"github.com/bishoku/vmcp/core/internal/testidp"
)

const claimSessionKey = "secure-session-test-key-32-bytes!!"

func claimSessionAPI(t *testing.T) (*api, *testidp.IdP) {
	t.Helper()
	idp := testidp.New(t)
	a := setup(t, func(s *controlplane.Server) {
		p, err := oidc.New(context.Background(), oidc.Config{Issuer: idp.URL(), ClientID: "control-client", RedirectURL: "http://localhost/callback", ViewerGroups: []string{"viewers"}, SessionKey: []byte(claimSessionKey), HTTPClient: idp.Srv.Client(), Sessions: oidc.PostgresSessions{Pool: s.Store.Pool, Box: s.Store.Box, Issuer: idp.URL()}, ClaimPaths: func(ctx context.Context) ([]string, error) {
			return store.LoadOIDCClaimPaths(ctx, s.Store.Pool, idp.URL())
		}, CustomLogin: func(ctx context.Context, issuer, subject string, claims map[string]any) (bool, error) {
			return store.CanOIDCLogin(ctx, s.Store.Pool, issuer, subject, claims)
		}})
		if err != nil {
			t.Fatal(err)
		}
		s.OIDC = p
	})
	return a, idp
}

// Trusted session fixture: verified-callback production is exercised separately
// in oidc tests. API tests exercise policy reload and the production store reader.
func claimSessionCookie(t *testing.T, a *api, issuer, role string, claims map[string]any, expired bool) string {
	t.Helper()
	var b [16]byte
	now := time.Now().UTC()
	sum := sha256.Sum256([]byte(time.Now().String()))
	copy(b[:], sum[:16])
	id := base64.RawURLEncoding.EncodeToString(b[:])
	deadline := now.Add(15 * time.Minute)
	if expired {
		deadline = now.Add(-time.Second)
	}
	s := oidc.Session{ID: id, Issuer: issuer, Version: 1, Subject: "browser-user", Name: "Browser User", Role: role, Expires: now.Add(8 * time.Hour), GroupsExpire: now.Add(15 * time.Minute), Claims: claims, ClaimsExpire: deadline, Groups: oidc.Groups(claims, "groups")}
	if err := (oidc.PostgresSessions{Pool: a.pool, Box: a.control.Store.Box, Issuer: issuer}).Save(context.Background(), s); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]any{"sid": id, "v": 1, "exp": s.Expires})
	body := base64.RawURLEncoding.EncodeToString(raw)
	mac := hmac.New(sha256.New, []byte(claimSessionKey))
	mac.Write([]byte("vmcp:server-session:v2\x00"))
	mac.Write(raw)
	return "vmcp_session_v2=" + body + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
func cookieHeaders(cookie string) map[string]string {
	return map[string]string{"Authorization": "", "Cookie": cookie}
}
func TestOIDCLiveMappingAuditModesFreshnessAndPolicyReload(t *testing.T) {
	a, idp := claimSessionAPI(t)
	role := assignmentRole(t, a, "session-auditor", "audit.events.read", "audit.legacy.events.read", "audit.legacy.payloads.read", "audit.payloads.request.read")
	input := mappingInput(idp.URL(), true)
	input["conditions"] = []map[string]any{{"path": "scope", "operator": "scope_contains", "value": "audit:read"}}
	m := assignmentCall(t, a, 201, "POST", "/api/v1/identity-mappings", input, "")
	grant := map[string]any{"roleId": role, "mappingId": m["id"], "scope": map[string]any{"kind": "tenant"}, "captureScopes": []string{"virtual"}, "enabled": true}
	assignmentCall(t, a, 201, "POST", "/api/v1/role-assignments", grant, "")
	req, res := payloadEvent(t, a, "request"), payloadEvent(t, a, "response")
	cookie := claimSessionCookie(t, a, idp.URL(), oidc.RoleViewer, map[string]any{"scope": "audit:read"}, false)
	code, _, out := a.do("GET", "/api/v1/audit-events/"+req+"/payload", nil, cookieHeaders(cookie))
	if code != 200 {
		t.Fatal("live verified mapping not resolved", code, out)
	}
	code, _, _ = a.do("GET", "/api/v1/audit-events/"+res+"/payload", nil, cookieHeaders(cookie))
	if code != 404 {
		t.Fatal("request grant opened response", code)
	}
	input["enabled"] = false
	assignmentCall(t, a, 200, "PUT", "/api/v1/identity-mappings/"+m["id"].(string), input, "\"1\"")
	code, _, _ = a.do("GET", "/api/v1/audit-events/"+req+"/payload", nil, cookieHeaders(cookie))
	if code != 404 {
		t.Fatal("mapping disable stale", code)
	}
	input["enabled"] = true
	assignmentCall(t, a, 200, "PUT", "/api/v1/identity-mappings/"+m["id"].(string), input, "\"2\"")
	expired := claimSessionCookie(t, a, idp.URL(), oidc.RoleViewer, map[string]any{"scope": "audit:read"}, true)
	code, _, _ = a.do("GET", "/api/v1/audit-events/"+req+"/payload", nil, cookieHeaders(expired))
	if code != 404 {
		t.Fatal("stale claim privilege", code)
	}
	// Direct subject grants outlive claim freshness, within the valid session TTL.
	direct := map[string]any{"roleId": role, "principal": map[string]any{"kind": "oidc", "issuer": idp.URL(), "id": "browser-user"}, "scope": map[string]any{"kind": "tenant"}, "captureScopes": []string{"virtual"}, "enabled": true}
	assignmentCall(t, a, 201, "POST", "/api/v1/role-assignments", direct, "")
	code, _, _ = a.do("GET", "/api/v1/audit-events/"+req+"/payload", nil, cookieHeaders(expired))
	if code != 200 {
		t.Fatal("direct assignment lost after claim TTL", code)
	}
	code, _, me := a.do("GET", "/api/v1/me", nil, cookieHeaders(expired))
	if code != 200 {
		t.Fatal(code, me)
	}
	encoded, _ := json.Marshal(me)
	if strings.Contains(string(encoded), "audit:read") || strings.Contains(string(encoded), "\"claims\"") {
		t.Fatal("claims returned to browser", string(encoded))
	}
	status := assignmentCall(t, a, 200, "GET", "/api/v1/access/status", nil, "")
	if status["claimMappings"] != "live-audit" || status["configurationEnforcement"] != nil {
		t.Fatal(status)
	}
}
func TestCustomSessionDoesNotInheritLegacyReadsOrWrites(t *testing.T) {
	a, idp := claimSessionAPI(t)
	cookie := claimSessionCookie(t, a, idp.URL(), oidc.RoleCustom, map[string]any{"scope": "audit:read"}, false)
	for _, path := range []string{"/api/v1/upstreams", "/api/v1/info", "/api/v1/roles", "/api/v1/access/status", "/api/v1/audit-events"} {
		code, _, _ := a.do("GET", path, nil, cookieHeaders(cookie))
		if code != 403 {
			t.Fatal("custom session inherited coarse GET", path, code)
		}
	}
	code, _, _ := a.do("GET", "/api/v1/me", nil, cookieHeaders(cookie))
	if code != 200 {
		t.Fatal("identity endpoint unavailable", code)
	}

	role := assignmentRole(t, a, "custom-session-auditor", "audit.events.read", "audit.legacy.events.read")
	input := mappingInput(idp.URL(), true)
	input["conditions"] = []map[string]any{{"path": "scope", "operator": "scope_contains", "value": "audit:read"}}
	m := assignmentCall(t, a, 201, "POST", "/api/v1/identity-mappings", input, "")
	assignmentCall(t, a, 201, "POST", "/api/v1/role-assignments", map[string]any{"roleId": role, "mappingId": m["id"], "scope": map[string]any{"kind": "tenant"}, "enabled": true}, "")
	code, _, out := a.do("GET", "/api/v1/audit-events", nil, cookieHeaders(cookie))
	if code != 200 {
		t.Fatal("custom audit list", code, out)
	}
	for _, path := range []string{"/api/v1/upstreams", "/api/v1/admin-tokens"} {
		code, _, _ = a.do("POST", path, map[string]any{}, cookieHeaders(cookie))
		if code != 403 {
			t.Fatal("missing CSRF/custom write accepted", path, code)
		}
	}
	headers := cookieHeaders(cookie)
	headers["X-VMCP-CSRF"] = "1"
	code, _, _ = a.do("POST", "/api/v1/upstreams", map[string]any{"slug": "blocked"}, headers)
	if code != 403 {
		t.Fatal("custom config write accepted", code)
	}
	// Admission follows current rollout and only grants a login with audit metadata rights.
	allowed, err := store.CanOIDCLogin(context.Background(), a.pool, idp.URL(), "browser-user", map[string]any{"scope": "audit:read"})
	if err != nil || !allowed {
		t.Fatal("custom sign-in decision", allowed, err)
	}

	allowed, err = store.CanOIDCLogin(context.Background(), a.pool, idp.URL(), "browser-user", map[string]any{"scope": "audit:read"})
	if err != nil || !allowed {
		t.Fatal("mandatory custom login", allowed, err)
	}
}
func TestNewMappingClaimRequiresNewSessionAndScopedRevoke(t *testing.T) {
	a, idp := claimSessionAPI(t)

	role := assignmentRole(t, a, "new-claim-auditor", "audit.events.read", "audit.legacy.events.read")
	input := mappingInput(idp.URL(), true)
	input["conditions"] = []map[string]any{{"path": "new.claim", "operator": "equals", "value": "sensitive"}}
	m := assignmentCall(t, a, 201, "POST", "/api/v1/identity-mappings", input, "")
	assignmentCall(t, a, 201, "POST", "/api/v1/role-assignments", map[string]any{"roleId": role, "mappingId": m["id"], "scope": map[string]any{"kind": "tenant"}, "enabled": true}, "")
	old := claimSessionCookie(t, a, idp.URL(), oidc.RoleCustom, map[string]any{}, false)
	code, _, _ := a.do("GET", "/api/v1/audit-events", nil, cookieHeaders(old))
	if code != 403 {
		t.Fatal("missing fresh claim granted", code)
	}
	fresh := claimSessionCookie(t, a, idp.URL(), oidc.RoleCustom, map[string]any{"new.claim": "sensitive"}, false)
	code, _, _ = a.do("GET", "/api/v1/audit-events", nil, cookieHeaders(fresh))
	if code != 200 {
		t.Fatal("fresh claim rejected", code)
	}
	paths, err := store.LoadOIDCClaimPaths(context.Background(), a.pool, idp.URL())
	if err != nil || len(paths) != 1 || paths[0] != "new.claim" {
		t.Fatal(paths, err)
	}
	// Other provider's same subject must not be revoked by the default request.
	other := claimSessionCookie(t, a, "https://another-provider", oidc.RoleCustom, nil, false)
	a.expect(204, "POST", "/api/v1/revocations/sessions", map[string]any{"subject": "browser-user"})
	code, _, _ = a.do("GET", "/api/v1/me", nil, cookieHeaders(fresh))
	if code != 401 {
		t.Fatal("revoke not observed", code)
	}
	var revoked bool
	raw := strings.TrimPrefix(other, "vmcp_session_v2=")
	part, _, _ := strings.Cut(raw, ".")
	decoded, _ := base64.RawURLEncoding.DecodeString(part)
	var ref map[string]any
	_ = json.Unmarshal(decoded, &ref)
	if err := a.pool.QueryRow(context.Background(), `SELECT revoked_at IS NOT NULL FROM browser_sessions WHERE id=$1`, ref["sid"]).Scan(&revoked); err != nil || revoked {
		t.Fatal("cross issuer revocation", revoked, err)
	}
	a.expect(204, "POST", "/api/v1/revocations/sessions", map[string]any{"subject": "browser-user", "issuer": "https://another-provider"})
	if err := a.pool.QueryRow(context.Background(), `SELECT revoked_at IS NOT NULL FROM browser_sessions WHERE id=$1`, ref["sid"]).Scan(&revoked); err != nil || !revoked {
		t.Fatal("explicit issuer revoke failed", revoked, err)
	}
	// The other issuer's record was never an authenticated session at this provider.
	code, _, _ = a.do("GET", "/api/v1/me", nil, cookieHeaders(other))
	if code != http.StatusUnauthorized {
		t.Fatal("issuer reuse accepted", code)
	}
}

func TestTeamLabelDoesNotOverrideExplicitSubjectAuthorization(t *testing.T) {
	a, idp := claimSessionAPI(t)
	role := assignmentRole(t, a, "independent-reader", "upstreams.read")
	a.expect(200, "POST", "/api/v1/teams", map[string]any{"slug": "original-team", "name": "Original team", "issuer": idp.URL(), "group": "original-members"})
	cookie := claimSessionCookie(t, a, idp.URL(), "team", map[string]any{"groups": []any{"original-members"}}, true)
	headers := cookieHeaders(cookie)
	code, _, _ := a.do("GET", "/api/v1/upstreams", nil, headers)
	if code != 403 {
		t.Fatalf("team label granted implicit rights: %d", code)
	}
	grant := assignmentCall(t, a, 201, "POST", "/api/v1/role-assignments", map[string]any{
		"roleId": role, "principal": map[string]any{"kind": "oidc", "issuer": idp.URL(), "id": "browser-user"},
		"scope": map[string]any{"kind": "tenant"}, "enabled": true}, "")
	code, _, me := a.do("GET", "/api/v1/me", nil, headers)
	if code != 200 || me["teamMember"] != true {
		t.Fatal("membership fixture", code, me)
	}
	// Ownership group changes do not revoke an independent subject assignment.
	if _, err := a.pool.Exec(context.Background(), `UPDATE teams SET group_value='replacement-members' WHERE slug='original-team'`); err != nil {
		t.Fatal(err)
	}
	code, _, out := a.do("GET", "/api/v1/upstreams", nil, headers)
	if code != 200 {
		t.Fatal("explicit grant rejected by obsolete team gate", code, out)
	}
	code, _, me = a.do("GET", "/api/v1/me", nil, headers)
	if code != 200 || me["teamMember"] == true {
		t.Fatal("stale team metadata", code, me)
	}
	code, _, _ = a.do("GET", "/api/v1/roles", nil, headers)
	if code != 403 {
		t.Fatal("team session became administrator", code)
	}
	assignmentCall(t, a, 204, "DELETE", "/api/v1/role-assignments/"+grant["id"].(string), nil, "\"1\"")
	code, _, _ = a.do("GET", "/api/v1/upstreams", nil, headers)
	if code != 403 {
		t.Fatal("revoked direct grant remained usable", code)
	}
}
