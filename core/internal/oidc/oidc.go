// Package oidc implements browser sign-in for the control plane UI (authorization code flow
// with PKCE) and maps the identity provider's group claim to a vMCP role.
//
// Production sessions use signed cookies backed by a shared revocation store.
package oidc

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/bishoku/vmcp/core/internal/agentauth"
	"github.com/bishoku/vmcp/core/internal/netpolicy"
)

// Roles, from least to most privileged.
const (
	RoleViewer = "viewer"
	RoleEditor = "editor"
	RoleAdmin  = "admin"
)

const (
	sessionCookie = "vmcp_session"
	flowCookie    = "vmcp_oauth"
	flowTTL       = 10 * time.Minute
)

// Config comes from environment variables (see README).
type Config struct {
	Issuer string
	// DiscoveryURL, when set, is where the discovery document is fetched from while Issuer stays
	// the public value tokens carry (split-horizon setups such as containers behind a public host name).
	DiscoveryURL string
	ClientID     string
	ClientSecret string
	RedirectURL  string
	// GroupsClaim names the ID token claim that lists the user's groups. Default "groups".
	GroupsClaim  string
	AdminGroups  []string
	EditorGroups []string
	ViewerGroups []string
	// SessionKey signs cookies; at least 32 bytes.
	SessionKey []byte
	SessionTTL time.Duration
	// HTTPClient is used for discovery and token exchange (tests only).
	HTTPClient *http.Client
	Sessions   SessionStore
}

// Session is the signed-in user.
type Session struct {
	ID      string    `json:"sid,omitempty"`
	Subject string    `json:"sub"`
	Name    string    `json:"name"`
	Role    string    `json:"role"`
	Expires time.Time `json:"exp"`
}

// Provider runs the sign-in flow.
type Provider struct {
	cfg        Config
	httpClient *http.Client
	endSession string
	oauth      oauth2.Config
	verifier   *gooidc.IDTokenVerifier
	secure     bool
	now        func() time.Time
}

// New contacts the issuer's discovery endpoint.
func New(ctx context.Context, cfg Config) (*Provider, error) {
	if cfg.Issuer == "" || cfg.ClientID == "" || cfg.RedirectURL == "" {
		return nil, errors.New("oidc: issuer, client id and redirect url are required")
	}
	if len(cfg.SessionKey) < 32 {
		return nil, errors.New("oidc: session key must be at least 32 bytes")
	}
	if len(cfg.AdminGroups)+len(cfg.EditorGroups)+len(cfg.ViewerGroups) == 0 {
		return nil, errors.New("oidc: configure at least one group-to-role mapping, otherwise nobody could sign in")
	}
	if cfg.GroupsClaim == "" {
		cfg.GroupsClaim = "groups"
	}
	if cfg.SessionTTL <= 0 {
		cfg.SessionTTL = 8 * time.Hour
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = netpolicy.Client(&http.Client{Timeout: 10 * time.Second})
	}
	ctx = gooidc.ClientContext(ctx, httpClient)
	from := cfg.Issuer
	if cfg.DiscoveryURL != "" {
		from = cfg.DiscoveryURL
		ctx = gooidc.InsecureIssuerURLContext(ctx, cfg.Issuer)
	}
	p, err := gooidc.NewProvider(ctx, from)
	if err != nil {
		return nil, fmt.Errorf("oidc discovery: %w", err)
	}
	var meta struct {
		EndSession string `json:"end_session_endpoint"`
	}
	_ = p.Claims(&meta)
	return &Provider{
		cfg:        cfg,
		httpClient: httpClient,
		endSession: meta.EndSession,
		oauth: oauth2.Config{
			ClientID: cfg.ClientID, ClientSecret: cfg.ClientSecret, Endpoint: p.Endpoint(),
			RedirectURL: cfg.RedirectURL, Scopes: []string{gooidc.ScopeOpenID, "profile", "email"},
		},
		verifier: p.Verifier(&gooidc.Config{ClientID: cfg.ClientID}),
		secure:   strings.HasPrefix(cfg.RedirectURL, "https://"),
		now:      time.Now,
	}, nil
}

type flow struct {
	State    string    `json:"s"`
	Nonce    string    `json:"n"`
	Verifier string    `json:"v"`
	Expires  time.Time `json:"exp"`
}

// Login starts the flow by redirecting to the identity provider.
func (p *Provider) Login(w http.ResponseWriter, r *http.Request) {
	f := flow{State: random(), Nonce: random(), Verifier: oauth2.GenerateVerifier(), Expires: p.now().Add(flowTTL)}
	p.setCookie(w, flowCookie, f, flowTTL)
	http.Redirect(w, r, p.oauth.AuthCodeURL(f.State, gooidc.Nonce(f.Nonce), oauth2.S256ChallengeOption(f.Verifier)), http.StatusFound)
}

// Callback completes the flow and sets the session cookie.
func (p *Provider) Callback(w http.ResponseWriter, r *http.Request) {
	var f flow
	if !p.readCookie(r, flowCookie, &f) || p.now().After(f.Expires) {
		http.Error(w, "sign-in expired, please try again", http.StatusBadRequest)
		return
	}
	p.clearCookie(w, flowCookie)
	if e := r.URL.Query().Get("error"); e != "" {
		http.Error(w, "identity provider refused the sign-in: "+e, http.StatusForbidden)
		return
	}
	if !hmac.Equal([]byte(r.URL.Query().Get("state")), []byte(f.State)) {
		http.Error(w, "invalid state", http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	if p.httpClient != nil {
		ctx = context.WithValue(ctx, oauth2.HTTPClient, p.httpClient)
		ctx = gooidc.ClientContext(ctx, p.httpClient)
	}
	tok, err := p.oauth.Exchange(ctx, r.URL.Query().Get("code"), oauth2.VerifierOption(f.Verifier))
	if err != nil {
		http.Error(w, "token exchange failed", http.StatusBadGateway)
		return
	}
	raw, _ := tok.Extra("id_token").(string)
	idt, err := p.verifier.Verify(ctx, raw)
	if err != nil || idt.Nonce != f.Nonce {
		http.Error(w, "invalid id token", http.StatusForbidden)
		return
	}
	var claims map[string]any
	if err := idt.Claims(&claims); err != nil {
		http.Error(w, "invalid id token", http.StatusForbidden)
		return
	}
	role := p.RoleFor(Groups(claims, p.cfg.GroupsClaim))
	if role == "" {
		http.Error(w, "your account is not in any group allowed to use vMCP", http.StatusForbidden)
		return
	}
	name, _ := claims["email"].(string)
	if n, _ := claims["name"].(string); n != "" {
		name = n
	}
	if name == "" {
		name = idt.Subject
	}
	s := Session{ID: random(), Subject: idt.Subject, Name: name, Role: role, Expires: p.now().Add(p.cfg.SessionTTL)}
	if p.cfg.Sessions != nil {
		if err := p.cfg.Sessions.Save(r.Context(), s); err != nil {
			http.Error(w, "session storage unavailable", http.StatusServiceUnavailable)
			return
		}
	}
	p.setCookie(w, sessionCookie, s, p.cfg.SessionTTL)
	http.Redirect(w, r, "/", http.StatusFound)
}

// EndSessionURL returns the identity provider's logout URL (RP-initiated logout) that brings the
// browser back to the UI afterwards, or "" when the provider does not advertise one.
func (p *Provider) EndSessionURL() string {
	if p.endSession == "" {
		return ""
	}
	u, err := url.Parse(p.cfg.RedirectURL)
	if err != nil {
		return ""
	}
	q := url.Values{"client_id": {p.cfg.ClientID}, "post_logout_redirect_uri": {u.Scheme + "://" + u.Host + "/"}}
	sep := "?"
	if strings.Contains(p.endSession, "?") {
		sep = "&"
	}
	return p.endSession + sep + q.Encode()
}

// Logout clears the session cookie.
func (p *Provider) Logout(w http.ResponseWriter, r *http.Request) bool {
	if p.cfg.Sessions != nil {
		var s Session
		if p.readCookie(r, sessionCookie, &s) && s.ID != "" {
			ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
			defer cancel()
			if err := p.cfg.Sessions.Revoke(ctx, s.ID); err != nil {
				http.Error(w, "session revocation unavailable", http.StatusServiceUnavailable)
				return false
			}
		}
	}
	p.clearCookie(w, sessionCookie)
	return true
}

// Session returns the signed-in user of a request, if any.
func (p *Provider) Session(r *http.Request) (Session, bool) {
	var s Session
	if !p.readCookie(r, sessionCookie, &s) || p.now().After(s.Expires) {
		return Session{}, false
	}
	if p.cfg.Sessions != nil {
		if s.ID == "" {
			return Session{}, false
		}
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		valid, err := p.cfg.Sessions.Valid(ctx, s.ID)
		if err != nil || !valid {
			return Session{}, false
		}
	}
	return s, true
}

// RoleFor returns the highest role granted by the groups, or "" when none applies.
func (p *Provider) RoleFor(groups []string) string {
	has := func(want []string) bool {
		for _, g := range groups {
			for _, w := range want {
				if g == w {
					return true
				}
			}
		}
		return false
	}
	switch {
	case has(p.cfg.AdminGroups):
		return RoleAdmin
	case has(p.cfg.EditorGroups):
		return RoleEditor
	case has(p.cfg.ViewerGroups):
		return RoleViewer
	}
	return ""
}

// Groups reads a string list (or a single string) from a claim. The claim may be a nested path
// such as "realm_access.roles" (Keycloak realm roles); an exact top-level key wins.
func Groups(claims map[string]any, claim string) []string {
	switch v := agentauth.Lookup(claims, claim).(type) {
	case []any:
		out := make([]string, 0, len(v))
		for _, g := range v {
			if s, ok := g.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case string:
		return []string{v}
	}
	return nil
}

func random() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func (p *Provider) sign(payload []byte) string {
	m := hmac.New(sha256.New, p.cfg.SessionKey)
	m.Write(payload)
	return base64.RawURLEncoding.EncodeToString(payload) + "." + base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

func (p *Provider) setCookie(w http.ResponseWriter, name string, v any, ttl time.Duration) {
	payload, _ := json.Marshal(v)
	http.SetCookie(w, &http.Cookie{
		Name: name, Value: p.sign(payload), Path: "/", MaxAge: int(ttl.Seconds()),
		HttpOnly: true, Secure: p.secure, SameSite: http.SameSiteLaxMode,
	})
}

func (p *Provider) clearCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: p.secure, SameSite: http.SameSiteLaxMode})
}

func (p *Provider) readCookie(r *http.Request, name string, dst any) bool {
	c, err := r.Cookie(name)
	if err != nil {
		return false
	}
	body, sig, ok := strings.Cut(c.Value, ".")
	if !ok {
		return false
	}
	payload, err := base64.RawURLEncoding.DecodeString(body)
	if err != nil {
		return false
	}
	if !hmac.Equal([]byte(p.sign(payload)), []byte(body+"."+sig)) {
		return false
	}
	return json.Unmarshal(payload, dst) == nil
}

// Issuer identifies the verified SSO authority in metadata-only audit records.
func (p *Provider) Issuer() string { return p.cfg.Issuer }
