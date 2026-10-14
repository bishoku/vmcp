package agentauth

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	gooidc "github.com/coreos/go-oidc/v3/oidc"

	"github.com/bishoku/vmcp/core/internal/netpolicy"
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
	Algs        []string
	Revocations TokenRevocations
	// HTTPClient overrides the default netpolicy-guarded client (tests only).
	HTTPClient *http.Client
}

// Identity is a verified caller.
type Identity struct {
	Subject string
	Claims  map[string]any
}

type cachedToken struct {
	identity  *Identity
	expiry    time.Time
	notBefore time.Time
	hasNBF    bool
}

// Verifier validates access tokens that are signed JWTs.
type Verifier struct {
	issuer      string
	audience    string
	httpClient  *http.Client
	verifier    *gooidc.IDTokenVerifier
	now         func() time.Time
	revocations TokenRevocations
	mu          sync.RWMutex
	verified    map[[32]byte]cachedToken
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
		return nil, fmt.Errorf("agent oidc discovery: %w", err)
	}
	return &Verifier{
		issuer:      cfg.Issuer,
		audience:    cfg.Audience,
		httpClient:  httpClient,
		verifier:    p.Verifier(&gooidc.Config{SkipClientIDCheck: true, SupportedSigningAlgs: algs}),
		now:         time.Now,
		revocations: cfg.Revocations,
		verified:    map[[32]byte]cachedToken{},
	}, nil
}

// Issuer is the issuer whose tokens are accepted.
func (v *Verifier) Issuer() string { return v.issuer }

// Verify checks signature, issuer, expiry, not-before and audience.
func (v *Verifier) Verify(ctx context.Context, raw string) (*Identity, error) {
	hash := sha256.Sum256([]byte(raw))
	if v.revocations != nil {
		check, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		revoked, err := v.revocations.Revoked(check, hash[:])
		if err != nil || revoked {
			return nil, ErrInvalidToken
		}
	}
	now := v.now()
	v.mu.RLock()
	cached, ok := v.verified[hash]
	v.mu.RUnlock()
	if ok {
		if !now.Before(cached.expiry) {
			v.mu.Lock()
			delete(v.verified, hash)
			v.mu.Unlock()
			return nil, fmt.Errorf("%w: token is expired", ErrInvalidToken)
		}
		if cached.hasNBF && cached.notBefore.After(now.Add(notBeforeSkew)) {
			return nil, fmt.Errorf("%w: not yet valid", ErrInvalidToken)
		}
		return cached.identity, nil
	}

	if v.httpClient != nil {
		ctx = gooidc.ClientContext(ctx, v.httpClient)
	}
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
	var nbfTime time.Time
	hasNBF := false
	if nbf, ok := claims["nbf"].(float64); ok {
		hasNBF = true
		nbfTime = time.Unix(int64(nbf), 0)
		if nbfTime.After(now.Add(notBeforeSkew)) {
			return nil, fmt.Errorf("%w: not yet valid", ErrInvalidToken)
		}
	}
	if t.Subject == "" {
		return nil, fmt.Errorf("%w: no subject", ErrInvalidToken)
	}
	id := &Identity{Subject: t.Subject, Claims: claims}
	if now.Before(t.Expiry) {
		v.mu.Lock()
		if len(v.verified) >= 4096 {
			for k, entry := range v.verified {
				if !now.Before(entry.expiry) {
					delete(v.verified, k)
				}
			}
			if len(v.verified) >= 4096 {
				clear(v.verified)
			}
		}
		if v.verified == nil {
			v.verified = map[[32]byte]cachedToken{}
		}
		v.verified[hash] = cachedToken{
			identity:  id,
			expiry:    t.Expiry,
			notBefore: nbfTime,
			hasNBF:    hasNBF,
		}
		v.mu.Unlock()
	}
	return id, nil
}

// LooksLikeJWT reports whether a bearer token has the three-part JWT shape.
func LooksLikeJWT(tok string) bool {
	return strings.Count(tok, ".") == 2 && !strings.HasPrefix(tok, "vmcp_")
}
