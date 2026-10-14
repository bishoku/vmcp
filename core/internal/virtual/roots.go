package virtual

import (
	"context"
	"encoding/json"
	"github.com/bishoku/vmcp/core/internal/upstream"
)

func (h *Handler) notifyRoots(ctx context.Context, sess *Session) {
	snap, snapshotErr := h.Snap.AuthorizedSnapshot()
	if snapshotErr != nil {
		return
	}
	vs, exists := snap.VirtualServers[sess.slug]
	if !exists || !vs.ClientFeatures.Roots || !subscriptionOwnerAllowed(sess.owner, vs, snap) {
		return
	}
	if h.Delivery == nil {
		return
	}
	var clientCaps struct {
		Roots *struct {
			ListChanged bool `json:"listChanged"`
		} `json:"roots"`
	}
	if json.Unmarshal(vs.ClientFeatures.Negotiated(sess.clientCapabilities), &clientCaps) != nil || clientCaps.Roots == nil || !clientCaps.Roots.ListChanged {
		return
	}
	sess.mu.Lock()
	connections := make([]*upstreamConn, 0, len(sess.conns))
	for _, conn := range sess.conns {
		connections = append(connections, conn)
	}
	sess.mu.Unlock()
	if len(connections) == 0 {
		return
	}
	var revision int64
	if err := h.Delivery.Pool.QueryRow(ctx, `SELECT roots_revision FROM mcp_client_notifications WHERE session_id=$1`, sess.id).Scan(&revision); err != nil || revision == 0 {
		return
	}
	for _, conn := range connections {
		conn.mu.Lock()
		cached := conn.current
		if cached == nil || cached.retired || cached.rootsRevision >= revision {
			conn.mu.Unlock()
			continue
		}
		var caps struct {
			Roots *struct {
				ListChanged bool `json:"listChanged"`
			} `json:"roots"`
		}
		_ = json.Unmarshal(cached.clientCapabilities, &caps)
		if caps.Roots == nil || !caps.Roots.ListChanged {
			conn.mu.Unlock()
			continue
		}
		cached.active++
		conn.mu.Unlock()
		err := upstream.Notify(ctx, cached.conn, "notifications/roots/list_changed", nil)
		conn.mu.Lock()
		if err == nil {
			cached.rootsRevision = revision
		}
		cached.active--
		conn.closeLocked(cached)
		conn.mu.Unlock()
	}
}
