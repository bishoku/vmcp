package config

import (
	"crypto/sha256"
	"encoding/json"
	"net/netip"
	"time"

	"github.com/bishoku/vmcp/core/internal/mcp"
	"github.com/bishoku/vmcp/core/internal/oauthtoken"
	"github.com/bishoku/vmcp/core/internal/pii"
	"github.com/bishoku/vmcp/core/internal/tlsconf"
)

// Snapshot is the immutable, versioned configuration the data plane serves from.
// The control plane produces it; the data plane swaps it in atomically.
type Snapshot struct {
	Revision       int64
	Upstreams      map[string]Upstream      // keyed by upstream ID
	VirtualServers map[string]VirtualServer // keyed by slug (single tenant for now)
	AgentKeys      map[string]AgentKey      // keyed by string(SHA-256 of the key)
	KeyLimits      map[string]int           // principal (agent key slug or "oauth:<binding>") -> calls per minute
	Bindings       []Binding                // identity bindings of enabled bindings, ordered by slug
}

// AgentKey authorizes an agent to use some virtual servers. Revoked keys are not loaded.
type AgentKey struct {
	ID        string
	Slug      string
	Allowed   []netip.Prefix // empty = any address
	ExpiresAt *time.Time
	Servers   map[string]bool // virtual server slugs
	// RateLimit is calls per minute; 0 = unlimited.
	RateLimit int
}

// Condition is one claim test of an identity binding; all conditions of a binding must hold.
type Condition struct {
	Claim string `json:"claim"`
	Op    string `json:"op"`
	Value string `json:"value,omitempty"`
}

// Binding authorizes holders of an IdP token whose claims match to use some virtual servers.
type Binding struct {
	ID         string
	Slug       string
	Conditions []Condition
	Allowed    []netip.Prefix // empty = any address
	Servers    map[string]bool
	RateLimit  int // calls per minute; 0 = unlimited
}

// Principal is the name used in audit, sessions and rate limits for a token holder.
func (b Binding) Principal() string { return "oauth:" + b.Slug }

// Upstream is a physical MCP server. Headers are already decrypted.
type Upstream struct {
	ID   string
	Slug string
	URL  string
	// FallbackURLs are tried in order when URL does not answer.
	FallbackURLs []string
	Headers      map[string]string
	// Transport is "streamable_http" or "sse".
	Transport string
	// TLS carries the custom CA / SNI / mTLS settings (zero = system defaults).
	TLS tlsconf.Settings
	// OAuth, when set, makes the gateway fetch client_credentials tokens for this upstream (secret decrypted).
	OAuth *oauthtoken.Config
}

// VirtualServer is an agent-facing endpoint exposing a filtered set of upstream tools.
type VirtualServer struct {
	Slug         string
	Name         string
	AuthRequired bool
	// RateLimit is calls per minute across all callers; 0 = unlimited.
	RateLimit int
	// PII is nil when masking is off.
	PII     *pii.Masker
	Tools   []VirtualTool
	Catalog []VirtualCatalogObject
	// Blocked maps exposed names of tools hidden by policy to the reason.
	Blocked map[string]string
}

// VirtualTool maps an exposed tool to its upstream origin.
// Tool.Name is the exposed (namespaced or aliased) name; UpstreamName is the original.
type VirtualTool struct {
	UpstreamID   string
	UpstreamName string
	Tool         mcp.Tool
}

// Lookup finds a tool by its exposed name.
func (v VirtualServer) Lookup(exposed string) (VirtualTool, bool) {
	for _, t := range v.Tools {
		if t.Tool.Name == exposed {
			return t, true
		}
	}
	return VirtualTool{}, false
}

// ExposedName returns the alias when set, otherwise "{upstreamSlug}__{tool}".
func ExposedName(upstreamSlug, tool, alias string) string {
	if alias != "" {
		return alias
	}
	return upstreamSlug + "__" + tool
}

// Fingerprint identifies every setting used to establish an upstream connection.
// Canonical JSON sorts header map keys. The digest must never be logged (it covers secrets).
func (u Upstream) Fingerprint() [32]byte {
	b, _ := json.Marshal(u)
	return sha256.Sum256(b)
}
