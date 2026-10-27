package virtual

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/bishoku/vmcp/core/internal/agentauth"
	"github.com/bishoku/vmcp/core/internal/audit"
	"github.com/bishoku/vmcp/core/internal/config"
	"github.com/bishoku/vmcp/core/internal/store"
)

// Owner binds a session to an authentication mode and immutable credential identity.
// Principal remains the human-readable audit/rate-limit label.
type Owner struct {
	Mode         string
	Principal    string
	CredentialID string
	Issuer       string
	Subject      string
}

type subjectKey struct{}

// withSubject carries the JWT subject of the caller to the audit event of a tool call.
func withSubject(ctx context.Context, subject string) context.Context {
	if subject == "" {
		return ctx
	}
	return context.WithValue(ctx, subjectKey{}, subject)
}

func subjectFrom(ctx context.Context) string {
	s, _ := ctx.Value(subjectKey{}).(string)
	return s
}

// authenticate checks the credentials of a request to vs: an agent key, or, when JWT auth is
// configured, an access token from the identity provider mapped through identity bindings. It
// returns the verified owner, or writes the failure response and returns ok=false.
// It runs on every request against a freshness-checked snapshot; expiry is checked locally.
func (h *Handler) authenticate(w http.ResponseWriter, r *http.Request, vs config.VirtualServer, snap *config.Snapshot) (Owner, bool) {
	if !vs.AuthRequired {
		return Owner{Mode: "anonymous"}, true
	}
	return h.authenticateRuntime(w, r, vs, snap, true)
}

// authenticateRuntime validates credentials and IP restrictions even for public Omni targets.
func (h *Handler) authenticateRuntime(w http.ResponseWriter, r *http.Request, vs config.VirtualServer, snap *config.Snapshot, requireAssignment bool) (Owner, bool) {
	deny := func(status int, msg, subject string, invalidToken bool) (Owner, bool) {
		h.record(audit.Event{VirtualServer: vs.Slug, ClientIP: h.ipString(r), Method: r.Method, Subject: subject, Status: "denied", Reason: msg})
		if status == http.StatusUnauthorized {
			ch := `Bearer realm="vmcp"`
			if h.JWT != nil {
				ch += `, resource_metadata="` + h.publicBase(r) + "/.well-known/oauth-protected-resource/mcp/v1/virtual/" + vs.Slug + `"`
				if invalidToken {
					ch += `, error="invalid_token"`
				}
			}
			w.Header().Set("WWW-Authenticate", ch)
		}
		http.Error(w, msg, status)
		return Owner{}, false
	}
	tok, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok || tok == "" {
		return deny(http.StatusUnauthorized, "agent key or access token required", "", false)
	}
	if h.JWT != nil && agentauth.LooksLikeJWT(tok) {
		id, err := h.JWT.Verify(r.Context(), tok)
		if err != nil {
			h.Log.Debug("access token rejected", "err", err)
			return deny(http.StatusUnauthorized, "invalid access token", "", true)
		}
		ip, ipKnown := h.clientIP(r)
		bindings := snap.Bindings
		if !requireAssignment {
			bindings = append([]config.Binding(nil), bindings...)
			for n := range bindings {
				bindings[n].Servers = map[string]bool{vs.Slug: true}
			}
		}
		b, outcome := agentauth.Resolve(bindings, id.Claims, vs.Slug, func(b config.Binding) bool {
			return len(b.Allowed) == 0 || (ipKnown && anyContains(b.Allowed, ip))
		})
		switch outcome {
		case agentauth.Allowed:
			return Owner{Mode: "oauth", Principal: b.Principal(), CredentialID: b.ID, Issuer: h.JWT.Issuer(), Subject: id.Subject}, true
		case agentauth.NoMatch:
			return deny(http.StatusForbidden, "no identity binding matches this token", id.Subject, false)
		case agentauth.ServerDenied:
			return deny(http.StatusForbidden, "token is not allowed on this virtual server", id.Subject, false)
		}
		return deny(http.StatusForbidden, "client address not allowed", id.Subject, false)
	}
	key, ok := snap.AgentKeys[string(store.HashKey(tok))]
	if !ok || (key.ExpiresAt != nil && !key.ExpiresAt.After(time.Now())) {
		return deny(http.StatusUnauthorized, "invalid agent key", "", false)
	}
	if requireAssignment && !key.Servers[vs.Slug] {
		return deny(http.StatusForbidden, "agent key is not allowed on this virtual server", "", false)
	}
	if len(key.Allowed) > 0 {
		ip, ok := h.clientIP(r)
		if !ok || !anyContains(key.Allowed, ip) {
			return deny(http.StatusForbidden, "client address not allowed", "", false)
		}
	}
	return Owner{Mode: "agent-key", Principal: key.Slug, CredentialID: key.ID}, true
}

// publicBase is the externally visible base URL of the data plane, for resource metadata.
func (h *Handler) publicBase(r *http.Request) string {
	if h.PublicURL != "" {
		return strings.TrimRight(h.PublicURL, "/")
	}
	scheme, host := "http", r.Host
	if r.TLS != nil {
		scheme = "https"
	}
	if peer, err := netip.ParseAddrPort(r.RemoteAddr); err == nil && anyContains(h.TrustedProxies, peer.Addr().Unmap()) {
		if v := r.Header.Get("X-Forwarded-Proto"); v == "http" || v == "https" {
			scheme = v
		}
		if v := r.Header.Get("X-Forwarded-Host"); v != "" {
			host = v
		}
	}
	return scheme + "://" + host
}

func anyContains(ps []netip.Prefix, ip netip.Addr) bool {
	for _, p := range ps {
		if p.Contains(ip) {
			return true
		}
	}
	return false
}

// clientIP returns the caller's address. X-Forwarded-For is honoured only when the
// direct peer is a trusted proxy; it is read right to left, stopping at the first
// address that is not itself a trusted proxy.
func (h *Handler) clientIP(r *http.Request) (netip.Addr, bool) {
	if ip, ok := r.Context().Value(verifiedOmniIPKey{}).(netip.Addr); ok {
		return ip, ip.IsValid()
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	peer, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}, false
	}
	peer = peer.Unmap()
	if !anyContains(h.TrustedProxies, peer) {
		return peer, true
	}
	parts := strings.Split(strings.Join(r.Header.Values("X-Forwarded-For"), ","), ",")
	for i := len(parts) - 1; i >= 0; i-- {
		a, err := netip.ParseAddr(strings.TrimSpace(parts[i]))
		if err != nil {
			return netip.Addr{}, false
		}
		a = a.Unmap()
		if !anyContains(h.TrustedProxies, a) {
			return a, true
		}
		peer = a
	}
	return peer, true
}

// resourceMetadata serves RFC 9728 protected-resource metadata so MCP clients can find the
// authorization server of a virtual server that accepts access tokens.
func (h *Handler) resourceMetadata(w http.ResponseWriter, r *http.Request) {
	vs, ok := h.Snap.Snapshot().VirtualServers[r.PathValue("slug")]
	if h.JWT == nil || !ok || !vs.AuthRequired {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"resource":                 h.publicBase(r) + "/mcp/v1/virtual/" + vs.Slug,
		"authorization_servers":    []string{h.JWT.Issuer()},
		"bearer_methods_supported": []string{"header"},
	})
}
