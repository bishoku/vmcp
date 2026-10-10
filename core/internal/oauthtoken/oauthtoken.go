// Package oauthtoken obtains and caches OAuth 2.0 client_credentials access tokens that the
// gateway presents to upstream MCP servers.
//
// One token is shared by every session and goroutine talking to the same upstream: concurrent
// callers wait on a single fetch, and the token is refreshed shortly before it expires.
package oauthtoken

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/bishoku/vmcp/core/internal/netpolicy"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
)

// Config describes how to obtain a token. ClientSecret is never logged or exported.
type Config struct {
	TokenURL     string
	ClientID     string
	ClientSecret string
	Scopes       []string
	// Audience is sent as the "audience" parameter (Auth0/Okta style); optional.
	Audience string
}

// Error is returned when a token cannot be obtained. It is distinct from errors of the upstream
// itself so callers do not treat an identity-provider outage as an unhealthy upstream.
type Error struct{ Err error }

func (e *Error) Error() string { return "upstream oauth: " + e.Err.Error() }
func (e *Error) Unwrap() error { return e.Err }

// IsTokenError reports whether err came from failing to obtain a token.
func IsTokenError(err error) bool {
	var te *Error
	return errors.As(err, &te)
}

const (
	refreshEarly = 30 * time.Second
	maxJitter    = 10 * time.Second
	// noExpiryTTL bounds tokens whose response carried no expires_in.
	noExpiryTTL  = 5 * time.Minute
	fetchTimeout = 15 * time.Second
)

// Source caches the token for one Config.
type Source struct {
	cfg  clientcredentials.Config
	http *http.Client
	now  func() time.Time

	mu      sync.Mutex
	token   string
	renewAt time.Time
	fetches int
}

// New returns an uncached Source (tests); production code uses For.
func New(c Config, hc *http.Client) *Source {
	if hc == nil {
		hc = &http.Client{Timeout: fetchTimeout}
	}
	cc := clientcredentials.Config{ClientID: c.ClientID, ClientSecret: c.ClientSecret, TokenURL: c.TokenURL, Scopes: c.Scopes,
		AuthStyle: oauth2.AuthStyleAutoDetect}
	if c.Audience != "" {
		cc.EndpointParams = url.Values{"audience": {c.Audience}}
	}
	return &Source{cfg: cc, http: netpolicy.Client(hc), now: time.Now}
}

// Token returns a valid access token, fetching one if needed.
func (s *Source) Token(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.token != "" && s.now().Before(s.renewAt) {
		return s.token, nil
	}
	return s.fetchLocked(ctx)
}

// Invalidate drops the cached token if it is still the given one (the upstream rejected it).
func (s *Source) Invalidate(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.token == token {
		s.token, s.renewAt = "", time.Time{}
	}
}

// Fetches reports how many times the token endpoint was called (tests).
func (s *Source) Fetches() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.fetches
}

func (s *Source) fetchLocked(ctx context.Context) (string, error) {
	// Detached from the caller: the result is shared, so one caller giving up must not fail the rest.
	fctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), fetchTimeout)
	defer cancel()
	fctx = context.WithValue(fctx, oauth2.HTTPClient, s.http)
	s.fetches++
	tok, err := s.cfg.Token(fctx)
	if err != nil {
		msg := err.Error()
		if s.cfg.ClientSecret != "" {
			msg = strings.ReplaceAll(msg, s.cfg.ClientSecret, "***") // an IdP may echo the secret back
		}
		return "", &Error{Err: fmt.Errorf("token request to %s failed: %s", s.cfg.TokenURL, msg)}
	}
	if tok.AccessToken == "" {
		return "", &Error{Err: errors.New("token endpoint returned no access_token")}
	}
	ttl := noExpiryTTL
	if !tok.Expiry.IsZero() {
		ttl = tok.Expiry.Sub(s.now())
	}
	margin := refreshEarly + time.Duration(rand.Int64N(int64(maxJitter)))
	if ttl <= 2*margin { // very short-lived: use most of its lifetime rather than refetching on every call
		margin = ttl / 4
	}
	s.token, s.renewAt = tok.AccessToken, s.now().Add(ttl-margin)
	return s.token, nil
}

var (
	regMu    sync.Mutex
	registry = map[string]*Source{}
)

// For returns the process-wide Source for this config, creating it on first use. Sources are
// keyed by a hash of the whole config, so changing the secret or scopes yields a fresh token.
func For(c Config) *Source {
	h := sha256.New()
	for _, p := range []string{c.TokenURL, c.ClientID, c.ClientSecret, c.Audience} {
		fmt.Fprintf(h, "%d:%s|", len(p), p)
	}
	for _, p := range c.Scopes {
		fmt.Fprintf(h, "s%d:%s|", len(p), p)
	}
	key := hex.EncodeToString(h.Sum(nil))
	regMu.Lock()
	defer regMu.Unlock()
	if s, ok := registry[key]; ok {
		return s
	}
	s := New(c, nil)
	registry[key] = s
	return s
}

// Transport wraps next so every request carries the bearer token. A 401 from the upstream
// invalidates the token and replays the request once with a fresh one.
func Transport(next http.RoundTripper, s *Source) http.RoundTripper {
	if next == nil {
		next = netpolicy.Client(nil).Transport
	}
	return &transport{next: next, src: s}
}

type transport struct {
	next http.RoundTripper
	src  *Source
}

func (t *transport) RoundTrip(req *http.Request) (*http.Response, error) {
	tok, err := t.src.Token(req.Context())
	if err != nil {
		return nil, err
	}
	resp, err := t.next.RoundTrip(withBearer(req, tok))
	if err != nil || resp.StatusCode != http.StatusUnauthorized {
		return resp, err
	}
	// Replay only if the body can be re-read.
	hasBody := req.Body != nil && req.Body != http.NoBody
	if hasBody && req.GetBody == nil {
		return resp, nil
	}
	t.src.Invalidate(tok)
	fresh, err := t.src.Token(req.Context())
	if err != nil {
		_ = resp.Body.Close()
		return nil, err
	}
	if fresh == tok {
		return resp, nil
	}
	retry := withBearer(req, fresh)
	if hasBody {
		b, err := req.GetBody()
		if err != nil {
			return resp, nil
		}
		retry.Body = b
	}
	_ = resp.Body.Close()
	return t.next.RoundTrip(retry)
}

func withBearer(req *http.Request, tok string) *http.Request {
	r := req.Clone(req.Context())
	r.Header.Set("Authorization", "Bearer "+tok)
	return r
}

// IsZero reports whether no OAuth client is configured.
func (c Config) IsZero() bool {
	return c.TokenURL == "" && c.ClientID == "" && c.ClientSecret == "" && len(c.Scopes) == 0 && c.Audience == ""
}

// Info is the read model of a Config: everything except the secret.
type Info struct {
	Type            string   `json:"type"` // "none" or "oauth_client_credentials"
	TokenURL        string   `json:"tokenUrl,omitempty"`
	ClientID        string   `json:"clientId,omitempty"`
	Scopes          []string `json:"scopes,omitempty"`
	Audience        string   `json:"audience,omitempty"`
	HasClientSecret bool     `json:"hasClientSecret,omitempty"`
}

// TypeClientCredentials is the Info.Type of a configured client.
const TypeClientCredentials = "oauth_client_credentials"

// Info describes the config without exposing the secret.
func (c Config) Info() Info {
	if c.IsZero() {
		return Info{Type: "none"}
	}
	return Info{Type: TypeClientCredentials, TokenURL: c.TokenURL, ClientID: c.ClientID, Scopes: c.Scopes, Audience: c.Audience,
		HasClientSecret: c.ClientSecret != ""}
}

// FieldError is a validation problem; Field is relative to the auth block (e.g. "tokenUrl").
type FieldError struct{ Field, Message string }

// Validate checks a non-zero config. http token URLs are allowed (private networks, dev) but
// send the client secret in clear text, which docs warn about.
func (c Config) Validate() []FieldError {
	var errs []FieldError
	if err := netpolicy.ValidateURL(c.TokenURL); err != nil {
		errs = append(errs, FieldError{"tokenUrl", "must be an absolute http(s) URL"})
	}
	if c.ClientID == "" {
		errs = append(errs, FieldError{"clientId", "is required"})
	}
	if c.ClientSecret == "" {
		errs = append(errs, FieldError{"clientSecret", "is required"})
	}
	for _, s := range c.Scopes {
		if s == "" || strings.ContainsAny(s, " \t\r\n\"\\") {
			errs = append(errs, FieldError{"scopes", fmt.Sprintf("invalid scope %q", s)})
		}
	}
	if len(c.Scopes) > 50 {
		errs = append(errs, FieldError{"scopes", "at most 50 scopes"})
	}
	return errs
}
