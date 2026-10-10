package virtual

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/bishoku/vmcp/core/internal/audit"
	"github.com/bishoku/vmcp/core/internal/config"
	"github.com/bishoku/vmcp/core/internal/store"
)

// authenticate checks the agent key for a request to vs. It returns the key slug
// ("" when the virtual server is open) or writes the failure response and returns ok=false.
// It runs on every request, so revocation and expiry take effect immediately.
func (h *Handler) authenticate(w http.ResponseWriter, r *http.Request, vs config.VirtualServer, snap *config.Snapshot) (string, bool) {
	if !vs.AuthRequired {
		return "", true
	}
	deny := func(status int, msg string) (string, bool) {
		h.record(audit.Event{VirtualServer: vs.Slug, ClientIP: h.ipString(r), Method: r.Method, Status: "denied", Reason: msg})
		if status == http.StatusUnauthorized {
			w.Header().Set("WWW-Authenticate", `Bearer realm="vmcp"`)
		}
		http.Error(w, msg, status)
		return "", false
	}
	tok, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok || tok == "" {
		return deny(http.StatusUnauthorized, "agent key required")
	}
	key, ok := snap.AgentKeys[string(store.HashKey(tok))]
	if !ok || (key.ExpiresAt != nil && !key.ExpiresAt.After(time.Now())) {
		return deny(http.StatusUnauthorized, "invalid agent key")
	}
	if !key.Servers[vs.Slug] {
		return deny(http.StatusForbidden, "agent key is not allowed on this virtual server")
	}
	if len(key.Allowed) > 0 {
		ip, ok := h.clientIP(r)
		if !ok || !anyContains(key.Allowed, ip) {
			return deny(http.StatusForbidden, "client address not allowed")
		}
	}
	return key.Slug, true
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
