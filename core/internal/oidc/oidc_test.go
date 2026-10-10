package oidc

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
)

// fakeIdP is a minimal OIDC provider: discovery, JWKS, authorize and token endpoints.
type fakeIdP struct {
	srv    *httptest.Server
	key    *rsa.PrivateKey
	claims map[string]any
	nonce  string
	code   string
	// issuer overrides the advertised issuer (split-horizon); endSession adds end_session_endpoint.
	issuer, endSession string
}

func (f *fakeIdP) iss() string {
	if f.issuer != "" {
		return f.issuer
	}
	return f.srv.URL
}

func newFakeIdP(t *testing.T) *fakeIdP {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeIdP{key: key, code: "the-code"}
	mux := http.NewServeMux()
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		doc := map[string]any{
			"issuer": f.iss(), "authorization_endpoint": f.srv.URL + "/authorize", "token_endpoint": f.srv.URL + "/token",
			"jwks_uri": f.srv.URL + "/jwks", "id_token_signing_alg_values_supported": []string{"RS256"},
		}
		if f.endSession != "" {
			doc["end_session_endpoint"] = f.endSession
		}
		_ = json.NewEncoder(w).Encode(doc)
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &key.PublicKey, KeyID: "k1", Algorithm: "RS256", Use: "sig"}}})
	})
	mux.HandleFunc("/authorize", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("code_challenge") == "" || q.Get("code_challenge_method") != "S256" {
			http.Error(w, "PKCE required", http.StatusBadRequest)
			return
		}
		f.nonce = q.Get("nonce")
		http.Redirect(w, r, q.Get("redirect_uri")+"?code="+f.code+"&state="+url.QueryEscape(q.Get("state")), http.StatusFound)
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("code") != f.code || r.Form.Get("code_verifier") == "" {
			http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
			return
		}
		signer, _ := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: key}, (&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", "k1"))
		claims := map[string]any{"nonce": f.nonce}
		for k, v := range f.claims {
			claims[k] = v
		}
		raw, err := jwt.Signed(signer).Claims(jwt.Claims{
			Issuer: f.iss(), Subject: "user-1", Audience: jwt.Audience{"vmcp"},
			Expiry: jwt.NewNumericDate(time.Now().Add(time.Hour)), IssuedAt: jwt.NewNumericDate(time.Now()),
		}).Claims(claims).Serialize()
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "at", "token_type": "Bearer", "id_token": raw})
	})
	return f
}

func provider(t *testing.T, f *fakeIdP) *Provider {
	t.Helper()
	p, err := New(context.Background(), Config{
		Issuer: f.srv.URL, ClientID: "vmcp", ClientSecret: "s", RedirectURL: "http://app.test/auth/callback",
		AdminGroups: []string{"admins"}, EditorGroups: []string{"devs"}, ViewerGroups: []string{"everyone"},
		SessionKey: make([]byte, 32),
	})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// signIn drives login -> idp -> callback and returns the response of the callback.
func signIn(t *testing.T, p *Provider, f *fakeIdP) (*httptest.ResponseRecorder, *http.Request) {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	base, _ := url.Parse("http://app.test/")

	rec := httptest.NewRecorder()
	p.Login(rec, httptest.NewRequest("GET", "/auth/login", nil))
	jar.SetCookies(base, rec.Result().Cookies())
	loc, _ := rec.Result().Location()

	// The "browser" follows the redirect to the IdP, which bounces back to the app.
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Get(loc.String())
	if err != nil {
		t.Fatal(err)
	}
	back, _ := resp.Location()
	resp.Body.Close()

	req := httptest.NewRequest("GET", "/auth/callback?"+back.RawQuery, nil)
	for _, c := range jar.Cookies(base) {
		req.AddCookie(c)
	}
	out := httptest.NewRecorder()
	p.Callback(out, req)
	return out, req
}

func TestSignInMapsGroupsToRole(t *testing.T) {
	cases := []struct {
		groups   any
		wantRole string
		wantCode int
	}{
		{[]string{"devs", "everyone"}, RoleEditor, 302},
		{[]string{"admins", "devs"}, RoleAdmin, 302},
		{[]string{"everyone"}, RoleViewer, 302},
		{"everyone", RoleViewer, 302},
		{[]string{"other"}, "", 403},
		{nil, "", 403},
	}
	for _, tc := range cases {
		f := newFakeIdP(t)
		f.claims = map[string]any{"groups": tc.groups, "email": "ada@example.com"}
		p := provider(t, f)
		rec, _ := signIn(t, p, f)
		if rec.Code != tc.wantCode {
			t.Fatalf("groups %v: status %d (%s)", tc.groups, rec.Code, rec.Body.String())
		}
		if tc.wantRole == "" {
			continue
		}
		req := httptest.NewRequest("GET", "/api/v1/me", nil)
		for _, c := range rec.Result().Cookies() {
			req.AddCookie(c)
		}
		s, ok := p.Session(req)
		if !ok || s.Role != tc.wantRole || s.Name != "ada@example.com" {
			t.Fatalf("groups %v: session %+v ok=%v", tc.groups, s, ok)
		}
	}
}

func TestSessionRejectsTampering(t *testing.T) {
	f := newFakeIdP(t)
	f.claims = map[string]any{"groups": []string{"everyone"}}
	p := provider(t, f)
	rec, _ := signIn(t, p, f)
	c := rec.Result().Cookies()[len(rec.Result().Cookies())-1]

	// Flip a byte of the payload.
	bad := *c
	bad.Value = "A" + c.Value[1:]
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&bad)
	if _, ok := p.Session(req); ok {
		t.Fatal("tampered cookie accepted")
	}
	// Expired session.
	p.now = func() time.Time { return time.Now().Add(9 * time.Hour) }
	req = httptest.NewRequest("GET", "/", nil)
	req.AddCookie(c)
	if _, ok := p.Session(req); ok {
		t.Fatal("expired session accepted")
	}
}

func TestCallbackRejectsWrongState(t *testing.T) {
	f := newFakeIdP(t)
	f.claims = map[string]any{"groups": []string{"admins"}}
	p := provider(t, f)
	_, req := signIn(t, p, f)
	q := req.URL.Query()
	q.Set("state", "forged")
	forged := httptest.NewRequest("GET", "/auth/callback?"+q.Encode(), nil)
	for _, c := range req.Cookies() {
		forged.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	p.Callback(rec, forged)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("forged state: %d", rec.Code)
	}
}

func TestNewRequiresMapping(t *testing.T) {
	f := newFakeIdP(t)
	_, err := New(context.Background(), Config{Issuer: f.srv.URL, ClientID: "vmcp", RedirectURL: "http://x/cb", SessionKey: make([]byte, 32)})
	if err == nil {
		t.Fatal("must refuse a config nobody could sign in with")
	}
}

func TestGroupsNestedClaimPath(t *testing.T) {
	claims := map[string]any{
		"realm_access": map[string]any{"roles": []any{"vmcp-admin", "x"}},
		"a.b":          []any{"exact"},
	}
	if g := Groups(claims, "realm_access.roles"); len(g) != 2 || g[0] != "vmcp-admin" {
		t.Fatalf("nested path: %v", g)
	}
	if g := Groups(claims, "a.b"); len(g) != 1 || g[0] != "exact" {
		t.Fatalf("exact key must win over a path: %v", g)
	}
	if g := Groups(claims, "realm_access.missing"); g != nil {
		t.Fatalf("missing path: %v", g)
	}
}

// The IdP is reached on one address (discovery URL) but tokens carry another (public issuer).
func TestSplitHorizonDiscovery(t *testing.T) {
	f := newFakeIdP(t)
	f.issuer = "https://sso.public.example/realms/vmcp"
	f.endSession = "https://sso.public.example/realms/vmcp/logout"
	f.claims = map[string]any{"groups": []string{"admins"}}
	cfg := Config{Issuer: f.issuer, DiscoveryURL: f.srv.URL, ClientID: "vmcp", ClientSecret: "s", RedirectURL: "http://app.test/auth/callback",
		AdminGroups: []string{"admins"}, SessionKey: make([]byte, 32)}
	if _, err := New(context.Background(), Config{Issuer: f.issuer, ClientID: "vmcp", RedirectURL: cfg.RedirectURL, AdminGroups: cfg.AdminGroups, SessionKey: cfg.SessionKey}); err == nil {
		t.Fatal("an unreachable public issuer must fail without a discovery URL")
	}
	p, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	rec, _ := signIn(t, p, f)
	if rec.Code != http.StatusFound {
		t.Fatalf("sign-in failed: %d %s", rec.Code, rec.Body.String())
	}
	req := httptest.NewRequest("GET", "/api/v1/me", nil)
	for _, c := range rec.Result().Cookies() {
		req.AddCookie(c)
	}
	if s, ok := p.Session(req); !ok || s.Role != RoleAdmin {
		t.Fatalf("session: %+v %v", s, ok)
	}
	u, _ := url.Parse(p.EndSessionURL())
	if u.Host != "sso.public.example" || u.Query().Get("client_id") != "vmcp" || u.Query().Get("post_logout_redirect_uri") != "http://app.test/" {
		t.Fatalf("end session url: %s", u)
	}
}
