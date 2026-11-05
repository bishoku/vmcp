package oidc

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bishoku/vmcp/core/internal/secrets"
	"github.com/bishoku/vmcp/core/internal/store"
	"github.com/bishoku/vmcp/core/internal/testdb"
)

func secureProvider(t *testing.T, f *fakeIdP) (*Provider, PostgresSessions) {
	t.Helper()
	ctx := context.Background()
	url := testdb.New(t)
	if err := store.Migrate(ctx, url); err != nil {
		t.Fatal(err)
	}
	pool, err := store.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	// Even a deployment configured to write legacy configuration envelopes must
	// write bound server session records, without altering the source box.
	box, err := secrets.NewKeyring("primary", map[string]string{"primary": base64.StdEncoding.EncodeToString(make([]byte, 32))}, true)
	if err != nil {
		t.Fatal(err)
	}
	db := PostgresSessions{Pool: pool, Box: box, Issuer: f.srv.URL}
	p := provider(t, f)
	p.cfg.Sessions = db
	p.cfg.ClaimPaths = func(context.Context) ([]string, error) {
		return []string{"groups", "scope", "organization.department"}, nil
	}
	return p, db
}
func secureRequest(t *testing.T, p *Provider, f *fakeIdP) *http.Request {
	t.Helper()
	rec, _ := signIn(t, p, f)
	if rec.Code != 302 {
		t.Fatalf("sign in %d %s", rec.Code, rec.Body.String())
	}
	req := httptest.NewRequest("GET", "/api/v1/me", nil)
	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == p.sessionCookieName() {
			req.AddCookie(cookie)
		}
	}
	return req
}
func TestEncryptedClaimSessionReferenceFreshnessAndRevocation(t *testing.T) {
	f := newFakeIdP(t)
	f.claims = map[string]any{"groups": []string{"everyone"}, "scope": "audit:read", "organization": map[string]any{"department": "security", "secret": "not-selected"}, "unselected": "private-token-like-value"}
	p, db := secureProvider(t, f)
	req := secureRequest(t, p, f)
	cookie, _ := req.Cookie("vmcp_session_v2")
	body, _, _ := bytes.Cut([]byte(cookie.Value), []byte("."))
	raw, err := base64.RawURLEncoding.DecodeString(string(body))
	if err != nil {
		t.Fatal(err)
	}
	var reference map[string]any
	if err := json.Unmarshal(raw, &reference); err != nil {
		t.Fatal(err)
	}
	if len(reference) != 3 || reference["sid"] == nil || reference["v"] != float64(1) {
		t.Fatal("cookie must be reference only", reference)
	}
	if bytes.Contains(raw, []byte("everyone")) || bytes.Contains(raw, []byte("alice")) || bytes.Contains(raw, []byte("audit:read")) {
		t.Fatal("identity leaked to cookie", string(raw))
	}
	s, ok := p.Session(req)
	if !ok || s.Claims["scope"] != "audit:read" || s.Claims["organization.department"] != "security" {
		t.Fatal("stored verified claims", s, ok)
	}
	if _, exists := s.Claims["unselected"]; exists {
		t.Fatal("unselected claim saved")
	}
	var encrypted []byte
	if err := db.Pool.QueryRow(context.Background(), `SELECT session_ciphertext FROM browser_sessions WHERE id=$1`, s.ID).Scan(&encrypted); err != nil {
		t.Fatal(err)
	}
	if _, bound := db.Box.KeyID(encrypted); !bound || bytes.Contains(encrypted, []byte("audit:read")) {
		t.Fatal("claim record not bound/encrypted")
	}
	now := p.now()
	p.now = func() time.Time { return now.Add(15 * time.Minute) }
	expired, ok := p.Session(req)
	if !ok || expired.Claims != nil || expired.Role != RoleViewer {
		t.Fatal("claim TTL changed legacy role", expired, ok)
	}
	// A second node loads the same authoritative record and observes revocation.
	other := &Provider{cfg: p.cfg, now: time.Now}
	if _, ok := other.Session(req); !ok {
		t.Fatal("cross-node session failed")
	}
	if !p.Logout(httptest.NewRecorder(), req) {
		t.Fatal("logout failed")
	}
	if _, ok := other.Session(req); ok {
		t.Fatal("revoked cross-node session accepted")
	}
}
func TestSecureSessionsRejectLegacyTamperingAndIssuerReuse(t *testing.T) {
	f := newFakeIdP(t)
	f.claims = map[string]any{"groups": []string{"everyone"}}
	p, db := secureProvider(t, f)
	req := secureRequest(t, p, f)
	s, ok := p.Session(req)
	if !ok {
		t.Fatal("initial session")
	}
	legacy := Session{ID: "legacy", Subject: s.Subject, Role: RoleAdmin, Expires: time.Now().Add(time.Hour)}
	if _, err := db.Pool.Exec(context.Background(), `INSERT INTO browser_sessions(id,subject,expires_at) VALUES($1,$2,$3)`, legacy.ID, legacy.Subject, legacy.Expires); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	p.setCookie(rec, sessionCookie, legacy, time.Hour)
	oldReq := httptest.NewRequest("GET", "/", nil)
	oldReq.AddCookie(rec.Result().Cookies()[0])
	if _, ok := p.Session(oldReq); ok {
		t.Fatal("legacy cookie accepted without reauthentication")
	}
	// A signed reference cookie cannot work on a cookie-only older reader.
	legacyReader := &Provider{cfg: Config{SessionKey: p.cfg.SessionKey}, now: time.Now}
	if _, ok := legacyReader.Session(req); ok {
		t.Fatal("new cookie accepted by old cookie name")
	}
	// Even deliberately renaming the signed reference must fail on an old
	// reader with the same signing key and a valid database revocation row.
	renamed := httptest.NewRequest("GET", "/", nil)
	refCookie, _ := req.Cookie("vmcp_session_v2")
	renamed.AddCookie(&http.Cookie{Name: sessionCookie, Value: refCookie.Value})
	legacyReader.cfg.Sessions = PostgresSessions{Pool: db.Pool}
	if _, ok := legacyReader.Session(renamed); ok {
		t.Fatal("v2 reference accepted after renaming to legacy cookie")
	}
	otherIssuer := db
	otherIssuer.Issuer = "https://different"
	if _, ok, err := otherIssuer.LoadSession(context.Background(), s.ID, "https://different"); err != nil || ok {
		t.Fatal("issuer boundary failed", ok, err)
	}
	// Swapping a valid ciphertext onto another ID fails authenticated binding.
	if _, err := db.Pool.Exec(context.Background(), `INSERT INTO browser_sessions(id,subject,issuer,expires_at,model_version,session_ciphertext) SELECT 'copied',subject,issuer,expires_at,model_version,session_ciphertext FROM browser_sessions WHERE id=$1`, s.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := db.LoadSession(context.Background(), "copied", db.Issuer); err == nil || ok {
		t.Fatal("cross-session ciphertext replay")
	}
	if _, err := db.Pool.Exec(context.Background(), `UPDATE browser_sessions SET subject='another-subject' WHERE id=$1`, s.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok := p.Session(req); ok {
		t.Fatal("row identity tampering accepted")
	}
}
func TestCustomOnlyLoginAndNewPathsRequireFreshSignIn(t *testing.T) {
	f := newFakeIdP(t)
	f.claims = map[string]any{"groups": []string{"custom-auditors"}, "scope": "audit:read", "new_claim": "new-value"}
	p, _ := secureProvider(t, f)
	p.cfg.CustomLogin = func(_ context.Context, issuer, subject string, claims map[string]any) (bool, error) {
		return issuer == f.srv.URL && subject != "" && claims["scope"] == "audit:read", nil
	}
	req := secureRequest(t, p, f)
	first, ok := p.Session(req)
	if !ok || first.Role != RoleCustom {
		t.Fatal(first, ok)
	}
	if _, exists := first.Claims["new_claim"]; exists {
		t.Fatal("unspecified claim stored")
	}
	p.cfg.ClaimPaths = func(context.Context) ([]string, error) { return []string{"scope", "new_claim"}, nil }
	old, ok := p.Session(req)
	if !ok || old.Claims["new_claim"] != nil {
		t.Fatal("new policy hydrated old session", old, ok)
	}
	newReq := secureRequest(t, p, f)
	fresh, ok := p.Session(newReq)
	if !ok || fresh.Claims["new_claim"] != "new-value" {
		t.Fatal("new sign-in missing required claim", fresh, ok)
	}
}
func TestCaptureClaimsTypedSubsetAndBounds(t *testing.T) {
	sample := map[string]any{"exact.key": "literal", "exact": map[string]any{"key": "nested"}, "nested": map[string]any{"selected": "ok", "secret": "discard"}, "object": map[string]any{"secret": "discard"}, "number": 42, "array": []any{"a", 42, true}}
	out, err := captureClaims(sample, []string{"exact.key", "nested.selected", "object", "number", "array"})
	if err != nil {
		t.Fatal(err)
	}
	if out["exact.key"] != "literal" || out["nested.selected"] != "ok" || out["object"] != nil || out["number"] != nil {
		t.Fatal(out)
	}
	raw, _ := json.Marshal(out)
	if bytes.Contains(raw, []byte("discard")) {
		t.Fatal(string(raw))
	}
	paths := make([]string, 257)
	if _, err := captureClaims(sample, paths); err == nil {
		t.Fatal("path bound missing")
	}
}
func TestIssuerScopedSubjectRevocation(t *testing.T) {
	f := newFakeIdP(t)
	f.claims = map[string]any{"groups": []string{"everyone"}}
	p, db := secureProvider(t, f)
	req := secureRequest(t, p, f)
	one, ok := p.Session(req)
	if !ok {
		t.Fatal("missing session")
	}
	other := db
	other.Issuer = "https://another-provider"
	two := one
	two.ID = random()
	two.Issuer = other.Issuer
	if err := other.Save(context.Background(), two); err != nil {
		t.Fatal(err)
	}
	if err := db.RevokeSubject(context.Background(), one.Subject); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := db.LoadSession(context.Background(), one.ID, one.Issuer); ok {
		t.Fatal("target not revoked")
	}
	if _, ok, err := other.LoadSession(context.Background(), two.ID, two.Issuer); err != nil || !ok {
		t.Fatal("other issuer revoked", ok, err)
	}
}

func TestClaimSessionCaptureOnlyAfterVerifiedIDToken(t *testing.T) {
	for _, bad := range []map[string]any{{"iss": "https://another-issuer"}, {"aud": "another-client"}, {"nonce": "another-nonce"}} {
		f := newFakeIdP(t)
		f.claims = map[string]any{"groups": []string{"everyone"}}
		for k, v := range bad {
			f.claims[k] = v
		}
		p, db := secureProvider(t, f)
		captured := false
		p.cfg.ClaimPaths = func(context.Context) ([]string, error) { captured = true; return []string{"groups"}, nil }
		rec, _ := signIn(t, p, f)
		if rec.Code != 403 || captured {
			t.Fatal("unverified token reached claim capture", bad, rec.Code, captured)
		}
		var count int
		if err := db.Pool.QueryRow(context.Background(), `SELECT count(*) FROM browser_sessions`).Scan(&count); err != nil || count != 0 {
			t.Fatal("unverified token created session", count, err)
		}
	}
}
