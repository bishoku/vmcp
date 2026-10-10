package agentauth

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
)

// Config selects the identity provider whose tokens are accepted.
type Config struct {
	// Issuer is the "iss" tokens carry.
	Issuer string
	// DiscoveryURL, when set, is where discovery and keys are fetched from (split-horizon setups).
	DiscoveryURL string
	// Audience must appear in the token's "aud". Required: accepting any token of an issuer would
	// let tokens meant for other applications call vMCP.
	Audience string
	// Algs are the accepted signing algorithms. Default RS256 and ES256. "none" and HMAC are refused.
	Algs []string
}

// Identity is a verified caller.
type Identity struct {
	Subject string
	Claims  map[string]any
}

// Verifier validates access tokens that are signed JWTs.
type Verifier struct {
	issuer   string
	audience string
	verifier *gooidc.IDTokenVerifier
	now      func() time.Time
}

// ErrInvalidToken is returned (wrapped) for any token that fails verification.
var ErrInvalidToken = errors.New("invalid token")

const notBeforeSkew = 30 * time.Second

// NewVerifier contacts the provider (discovery) once; signing keys are fetched lazily and cached.
func NewVerifier(ctx context.Context, cfg Config) (*Verifier, error) {
	if cfg.Issuer == "" {
		return nil, errors.New("agent oidc: issuer is required")
	}
	if cfg.Audience == "" {
		return nil, errors.New("agent oidc: audience is required (VMCP_AGENT_OIDC_AUDIENCE)")
	}
	algs := cfg.Algs
	if len(algs) == 0 {
		algs = []string{"RS256", "ES256"}
	}
	for _, a := range algs {
		if a == "none" || strings.HasPrefix(a, "HS") {
			return nil, fmt.Errorf("agent oidc: algorithm %q is not allowed", a)
		}
	}
	from := cfg.Issuer
	if cfg.DiscoveryURL != "" {
		from = cfg.DiscoveryURL
		ctx = gooidc.InsecureIssuerURLContext(ctx, cfg.Issuer)
	}
	p, err := gooidc.NewProvider(ctx, from)
	if err != nil {
		return nil, fmt.Errorf("agent oidc discovery: %w", err)
	}
	return &Verifier{
		issuer:   cfg.Issuer,
		audience: cfg.Audience,
		verifier: p.Verifier(&gooidc.Config{SkipClientIDCheck: true, SupportedSigningAlgs: algs}),
		now:      time.Now,
	}, nil
}

// Issuer is the issuer whose tokens are accepted.
func (v *Verifier) Issuer() string { return v.issuer }

// Verify checks signature, issuer, expiry, not-before and audience.
func (v *Verifier) Verify(ctx context.Context, raw string) (*Identity, error) {
	t, err := v.verifier.Verify(ctx, raw)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}
	if !slices.Contains(t.Audience, v.audience) {
		return nil, fmt.Errorf("%w: audience", ErrInvalidToken)
	}
	var claims map[string]any
	if err := t.Claims(&claims); err != nil {
		return nil, fmt.Errorf("%w: claims", ErrInvalidToken)
	}
	if nbf, ok := claims["nbf"].(float64); ok && time.Unix(int64(nbf), 0).After(v.now().Add(notBeforeSkew)) {
		return nil, fmt.Errorf("%w: not yet valid", ErrInvalidToken)
	}
	if t.Subject == "" {
		return nil, fmt.Errorf("%w: no subject", ErrInvalidToken)
	}
	return &Identity{Subject: t.Subject, Claims: claims}, nil
}

// LooksLikeJWT reports whether a bearer token has the three-part JWT shape.
func LooksLikeJWT(tok string) bool {
	return strings.Count(tok, ".") == 2 && !strings.HasPrefix(tok, "vmcp_")
}
