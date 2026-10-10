// Package testidp is a minimal OIDC identity provider (discovery, JWKS) that mints access tokens.
// It exists for tests only.
package testidp

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
)

// IdP serves discovery and keys and signs tokens with its key.
type IdP struct {
	Srv *httptest.Server
	Key *rsa.PrivateKey
	// Issuer is what discovery advertises and tokens carry; defaults to the server URL.
	Issuer string
}

// New starts an IdP that stops with the test.
func New(t testing.TB) *IdP {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	i := &IdP{Key: key}
	mux := http.NewServeMux()
	i.Srv = httptest.NewServer(mux)
	t.Cleanup(i.Srv.Close)
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer": i.iss(), "authorization_endpoint": i.Srv.URL + "/authorize", "token_endpoint": i.Srv.URL + "/token",
			"jwks_uri": i.Srv.URL + "/jwks", "id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &key.PublicKey, KeyID: "k1", Algorithm: "RS256", Use: "sig"}}})
	})
	return i
}

func (i *IdP) iss() string {
	if i.Issuer != "" {
		return i.Issuer
	}
	return i.Srv.URL
}

// URL is the address of the IdP.
func (i *IdP) URL() string { return i.Srv.URL }

// Token describes a token to mint. Zero values give a valid token.
type Token struct {
	Subject   string
	Audience  []string
	Issuer    string
	Expiry    time.Time // default: in one hour
	NotBefore time.Time // default: unset
	KeyID     string    // default: "k1"
	Claims    map[string]any
}

// Mint signs a token with the IdP's key (RS256).
func (i *IdP) Mint(t testing.TB, tok Token) string {
	t.Helper()
	if tok.Subject == "" {
		tok.Subject = "agent-1"
	}
	if tok.Audience == nil {
		tok.Audience = []string{"vmcp-gateway"}
	}
	if tok.Issuer == "" {
		tok.Issuer = i.iss()
	}
	if tok.Expiry.IsZero() {
		tok.Expiry = time.Now().Add(time.Hour)
	}
	kid := tok.KeyID
	if kid == "" {
		kid = "k1"
	}
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: i.Key}, (&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", kid))
	if err != nil {
		t.Fatal(err)
	}
	std := jwt.Claims{Issuer: tok.Issuer, Subject: tok.Subject, Audience: tok.Audience,
		Expiry: jwt.NewNumericDate(tok.Expiry), IssuedAt: jwt.NewNumericDate(time.Now())}
	if !tok.NotBefore.IsZero() {
		std.NotBefore = jwt.NewNumericDate(tok.NotBefore)
	}
	raw, err := jwt.Signed(signer).Claims(std).Claims(tok.Claims).Serialize()
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
