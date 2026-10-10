package agentauth

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"

	"github.com/bishoku/vmcp/core/internal/config"
	"github.com/bishoku/vmcp/core/internal/testidp"
)

func verifier(t *testing.T, idp *testidp.IdP) *Verifier {
	t.Helper()
	v, err := NewVerifier(context.Background(), Config{Issuer: idp.Issuer, DiscoveryURL: idp.URL(), Audience: "vmcp-gateway"})
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestVerifier(t *testing.T) {
	idp := testidp.New(t)
	idp.Issuer = "https://sso.example/realms/x" // public issuer differs from the address it is served on
	v := verifier(t, idp)
	ctx := context.Background()

	id, err := v.Verify(ctx, idp.Mint(t, testidp.Token{Subject: "billing-bot", Claims: map[string]any{"azp": "billing-bot"}}))
	if err != nil || id.Subject != "billing-bot" || id.Claims["azp"] != "billing-bot" {
		t.Fatalf("valid token rejected: %v %+v", err, id)
	}

	bad := map[string]string{
		"wrong issuer":     idp.Mint(t, testidp.Token{Issuer: "https://evil.example"}),
		"wrong audience":   idp.Mint(t, testidp.Token{Audience: []string{"account"}}),
		"expired":          idp.Mint(t, testidp.Token{Expiry: time.Now().Add(-time.Minute)}),
		"not yet valid":    idp.Mint(t, testidp.Token{NotBefore: time.Now().Add(10 * time.Minute)}),
		"unknown key id":   idp.Mint(t, testidp.Token{KeyID: "other"}),
		"garbage":          "a.b.c",
		"unsigned (none)":  none(t),
		"hmac confusion":   hmacToken(t),
		"tampered payload": tamper(idp.Mint(t, testidp.Token{})),
	}
	for name, tok := range bad {
		if _, err := v.Verify(ctx, tok); !errors.Is(err, ErrInvalidToken) {
			t.Errorf("%s: accepted or wrong error: %v", name, err)
		}
	}
}

func none(t *testing.T) string {
	h := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	p := base64.RawURLEncoding.EncodeToString([]byte(`{"iss":"https://sso.example/realms/x","sub":"x","aud":"vmcp-gateway","exp":4102444800}`))
	return h + "." + p + "."
}

func hmacToken(t *testing.T) string {
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.HS256, Key: []byte("0123456789abcdef0123456789abcdef")}, (&jose.SignerOptions{}).WithType("JWT"))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := jwt.Signed(signer).Claims(jwt.Claims{Issuer: "https://sso.example/realms/x", Subject: "x", Audience: jwt.Audience{"vmcp-gateway"},
		Expiry: jwt.NewNumericDate(time.Now().Add(time.Hour))}).Serialize()
	return raw
}

func tamper(tok string) string {
	parts := strings.Split(tok, ".")
	p, _ := base64.RawURLEncoding.DecodeString(parts[1])
	p = []byte(strings.Replace(string(p), "agent-1", "admin-1", 1))
	parts[1] = base64.RawURLEncoding.EncodeToString(p)
	return strings.Join(parts, ".")
}

func TestVerifierConfig(t *testing.T) {
	idp := testidp.New(t)
	if _, err := NewVerifier(context.Background(), Config{Issuer: idp.URL()}); err == nil {
		t.Error("audience must be required")
	}
	for _, a := range []string{"none", "HS256"} {
		if _, err := NewVerifier(context.Background(), Config{Issuer: idp.URL(), Audience: "x", Algs: []string{a}}); err == nil {
			t.Errorf("algorithm %s must be refused", a)
		}
	}
}

func TestMatch(t *testing.T) {
	claims := map[string]any{
		"azp":          "billing-bot",
		"groups":       []any{"team-a", "team-b"},
		"scope":        "openid mcp.read mcp.write",
		"realm_access": map[string]any{"roles": []any{"agent"}},
		"empty":        "",
	}
	c := func(claim, op, value string) config.Condition {
		return config.Condition{Claim: claim, Op: op, Value: value}
	}
	cases := []struct {
		name  string
		conds []config.Condition
		want  bool
	}{
		{"equals", []config.Condition{c("azp", "equals", "billing-bot")}, true},
		{"equals miss", []config.Condition{c("azp", "equals", "other")}, false},
		{"equals on list is false", []config.Condition{c("groups", "equals", "team-a")}, false},
		{"contains list", []config.Condition{c("groups", "contains", "team-b")}, true},
		{"contains scope", []config.Condition{c("scope", "contains", "mcp.write")}, true},
		{"contains scope miss", []config.Condition{c("scope", "contains", "mcp")}, false},
		{"dotted path", []config.Condition{c("realm_access.roles", "contains", "agent")}, true},
		{"exists", []config.Condition{c("azp", "exists", "")}, true},
		{"exists empty string", []config.Condition{c("empty", "exists", "")}, false},
		{"exists missing", []config.Condition{c("nope", "exists", "")}, false},
		{"all must hold", []config.Condition{c("azp", "equals", "billing-bot"), c("groups", "contains", "team-z")}, false},
		{"and", []config.Condition{c("azp", "equals", "billing-bot"), c("groups", "contains", "team-a")}, true},
		{"no conditions never match", nil, false},
		{"unknown op", []config.Condition{c("azp", "regex", ".*")}, false},
	}
	for _, tc := range cases {
		if got := Match(tc.conds, claims); got != tc.want {
			t.Errorf("%s: got %v want %v", tc.name, got, tc.want)
		}
	}
}

func TestResolveOutcomes(t *testing.T) {
	claims := map[string]any{"azp": "bot"}
	match := []config.Condition{{Claim: "azp", Op: "equals", Value: "bot"}}
	bs := []config.Binding{
		{Slug: "a", Conditions: match, Servers: map[string]bool{"one": true}},
		{Slug: "b", Conditions: match, Servers: map[string]bool{"two": true}},
		{Slug: "c", Conditions: []config.Condition{{Claim: "azp", Op: "equals", Value: "other"}}, Servers: map[string]bool{"three": true}},
	}
	if b, o := Resolve(bs, claims, "two", nil); o != Allowed || b.Slug != "b" {
		t.Errorf("union across bindings: %v %v", b, o)
	}
	if _, o := Resolve(bs, claims, "three", nil); o != ServerDenied {
		t.Errorf("server of a non-matching binding: %v", o)
	}
	if _, o := Resolve(bs, map[string]any{"azp": "x"}, "one", nil); o != NoMatch {
		t.Errorf("no match: %v", o)
	}
	if _, o := Resolve(bs, claims, "one", func(config.Binding) bool { return false }); o != IPDenied {
		t.Errorf("ip denied: %v", o)
	}
}
