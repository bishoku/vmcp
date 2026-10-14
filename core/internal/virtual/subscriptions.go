package virtual

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/bishoku/vmcp/core/internal/audit"
	"github.com/bishoku/vmcp/core/internal/config"
	"github.com/bishoku/vmcp/core/internal/delivery"
	"github.com/bishoku/vmcp/core/internal/mcp"
	"github.com/bishoku/vmcp/core/internal/upstream"
	"time"
)

// RunSubscriptions uses shared leases. Each replica has 32 workers, with
// a cluster-wide admission cap of 32 subscriptions and 16 per session.
func (h *Handler) RunSubscriptions(ctx context.Context) {
	if h.Delivery == nil {
		return
	}
	for i := 0; i < 32; i++ {
		go func() {
			tick := time.NewTicker(time.Second)
			defer tick.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-tick.C:
					h.runSubscription(ctx)
				}
			}
		}()
	}
}
func (h *Handler) subscription(ctx context.Context, sess *Session, object config.VirtualCatalogObject, exposed, original string, revision int64, subscribe bool) error {
	if h.Delivery == nil || !object.Subscribe {
		return errors.New("resource subscriptions unavailable")
	}
	if !subscribe {
		_, err := h.Delivery.Pool.Exec(ctx, `DELETE FROM mcp_resource_subscriptions WHERE session_id=$1 AND uri=$2`, sess.id, exposed)
		return err
	}
	tx, err := h.Delivery.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(19840924)`); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM mcp_resource_subscriptions WHERE expires_at<now()`); err != nil {
		return err
	}
	var all, own int
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE session_id=$1),COALESCE(bool_or(session_id=$1 AND uri=$2),false) FROM mcp_resource_subscriptions`, sess.id, exposed).Scan(&all, &own, &exists); err != nil {
		return err
	}
	if !exists && (all >= 32 || own >= 16) {
		return delivery.ErrCapacity
	}
	if _, err := tx.Exec(ctx, `INSERT INTO mcp_resource_subscriptions(session_id,uri,upstream_id,original_uri,revision) VALUES($1,$2,$3,$4,$5) ON CONFLICT(session_id,uri) DO UPDATE SET expires_at=now()+interval '15 minutes',failed=false`, sess.id, exposed, object.UpstreamID, original, revision); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	wait, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	wake, unsubscribe := h.Delivery.Subscribe(sess.id)
	defer unsubscribe()
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	for {
		var ready, failed bool
		if err := h.Delivery.Pool.QueryRow(wait, `SELECT ready,failed FROM mcp_resource_subscriptions WHERE session_id=$1 AND uri=$2`, sess.id, exposed).Scan(&ready, &failed); err != nil {
			return err
		}
		if failed {
			return errors.New("physical subscription failed")
		}
		if ready {
			return nil
		}
		select {
		case <-wait.Done():
			return wait.Err()
		case <-wake:
		case <-tick.C:
		}
	}
}
func (h *Handler) runSubscription(parent context.Context) {
	owner := delivery.UUID()
	var sid, uri, upID, original string
	claim, cancel := context.WithTimeout(parent, time.Second)
	err := h.Delivery.Pool.QueryRow(claim, `UPDATE mcp_resource_subscriptions s SET owner=$1,lease_until=now()+interval '15 seconds',ready=false FROM (SELECT session_id,uri FROM mcp_resource_subscriptions WHERE expires_at>now() AND NOT failed AND (lease_until IS NULL OR lease_until<now()) ORDER BY session_id,uri LIMIT 1 FOR UPDATE SKIP LOCKED) q WHERE s.session_id=q.session_id AND s.uri=q.uri RETURNING s.session_id,s.uri,s.upstream_id::text,s.original_uri`, owner).Scan(&sid, &uri, &upID, &original)
	cancel()
	if err != nil {
		return
	}
	ctx, stop := context.WithTimeout(parent, 15*time.Minute)
	defer stop()
	release := func(failed bool) {
		bounded, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_, _ = h.Delivery.Pool.Exec(bounded, `UPDATE mcp_resource_subscriptions SET owner=NULL,lease_until=NULL,ready=false,failed=$4 WHERE session_id=$1 AND uri=$2 AND owner=$3`, sid, uri, owner, failed)
		h.Delivery.NotifyLocal(sid)
	}
	snap, err := h.Snap.AuthorizedSnapshot()
	if err != nil {
		release(false)
		return
	}
	sess, err := h.Sessions.Resolve(ctx, sid)
	if err != nil || sess == nil {
		release(true)
		return
	}
	vs, ok := snap.VirtualServers[sess.slug]
	object, mapped, allowed := catalogRoute(vs, "resource", uri, true)
	up, found := snap.Upstreams[upID]
	if !ok || !allowed || !found || !object.Subscribe || !subscriptionOwnerAllowed(sess.owner, vs, snap) || object.UpstreamID != upID || mapped != original {
		release(true)
		return
	}
	// Renew during initialization too: a slow handshake cannot outlive its fence.
	initCtx, stopInitHeartbeat := context.WithCancel(ctx)
	heartbeatDone := make(chan struct{})
	go func() {
		defer close(heartbeatDone)
		tick := time.NewTicker(time.Second)
		defer tick.Stop()
		for {
			select {
			case <-initCtx.Done():
				return
			case <-tick.C:
				bounded, cancel := context.WithTimeout(ctx, time.Second)
				tag, err := h.Delivery.Pool.Exec(bounded, `UPDATE mcp_resource_subscriptions SET lease_until=now()+interval '15 seconds' WHERE session_id=$1 AND uri=$2 AND owner=$3 AND lease_until>now() AND expires_at>now()`, sid, uri, owner)
				cancel()
				if err != nil || tag.RowsAffected() != 1 {
					stop()
					return
				}
			}
		}
	}()
	defer func() { stopInitHeartbeat(); stop(); <-heartbeatDone }()
	recorder := audit.NewDurable(h.Delivery.Pool, h.Log)
	event, err := recorder.Admit(ctx, audit.Event{Time: time.Now().UTC(), ActorType: "system", ActorID: "resource-subscription", Action: "mcp.subscription_reconcile", Method: "resources/subscribe", VirtualServer: sess.slug, Revision: snap.Revision, Target: sid, Upstream: up.Slug})
	if err != nil {
		release(false)
		return
	}
	event.Status = "error"
	event.Reason = "subscription activation failed"
	defer func() {
		if ctx.Err() != nil {
			event.Status = "error"
			event.Reason = "subscription worker stopped"
		}
		bounded, cancel := context.WithTimeout(context.Background(), audit.WriteTimeout)
		defer cancel()
		_ = recorder.Complete(bounded, event)
	}()
	c, err := upstream.OpenAuth(up.Transport, up.URL, up.Headers, up.TLS, up.OAuth)
	if err != nil {
		release(true)
		return
	}
	defer func() {
		bounded, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_, _ = upstream.Call(bounded, c, "resources/unsubscribe", map[string]string{"uri": original})
		_ = c.Close(bounded)
	}()
	upstream.SetNotificationHandler(c, func(message *mcp.Message) {
		if message.Method != "notifications/resources/updated" {
			return
		}
		var p struct {
			URI string `json:"uri"`
		}
		if json.Unmarshal(message.Params, &p) != nil || p.URI != original {
			return
		}
		bounded, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		_ = h.Delivery.ResourceUpdated(bounded, sid, uri, owner)
	})
	handshakeCtx, initCancel := context.WithTimeout(ctx, 10*time.Second)
	init, err := c.Initialize(handshakeCtx)
	initCancel()
	if err != nil || init.Capabilities.Resources == nil || !init.Capabilities.Resources.Subscribe {
		release(true)
		return
	}
	watching := make(chan error, 1)
	go func() { watching <- upstream.Watch(ctx, c) }()
	callCtx, callCancel := context.WithTimeout(ctx, 10*time.Second)
	_, err = upstream.Call(callCtx, c, "resources/subscribe", map[string]string{"uri": original})
	callCancel()
	if err != nil {
		release(true)
		return
	}
	if _, err := h.Delivery.Pool.Exec(ctx, `UPDATE mcp_resource_subscriptions SET ready=true WHERE session_id=$1 AND uri=$2 AND owner=$3 AND lease_until>now()`, sid, uri, owner); err != nil {
		release(false)
		return
	}
	_, _ = h.Delivery.Pool.Exec(ctx, `SELECT pg_notify('vmcp_delivery',$1)`, sid)
	h.Delivery.NotifyLocal(sid)
	// Handshake is complete; the main watch loop below now owns the 1s lease renewal.
	stopInitHeartbeat()
	<-heartbeatDone
	event.Status = "ok"
	event.Reason = ""
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			release(false)
			return
		case watchErr := <-watching:
			event.Status = "error"
			event.Reason = "subscription stream interrupted"
			release(errors.Is(watchErr, upstream.ErrSubscriptionTransport))
			return
		case <-tick.C:
			bounded, cancel := context.WithTimeout(ctx, time.Second)
			live, resolveErr := h.Sessions.Resolve(bounded, sid)
			fresh, freshErr := h.Snap.AuthorizedSnapshot()
			valid := resolveErr == nil && live != nil && freshErr == nil
			if valid {
				v, ok := fresh.VirtualServers[live.slug]
				o, mapped, selected := catalogRoute(v, "resource", uri, true)
				current, exists := fresh.Upstreams[upID]
				valid = ok && subscriptionOwnerAllowed(live.owner, v, fresh) && selected && o.Subscribe && o.UpstreamID == upID && mapped == original && exists && current.Fingerprint() == up.Fingerprint()
			}
			if !valid {
				cancel()
				release(true)
				return
			}
			tag, err := h.Delivery.Pool.Exec(bounded, `UPDATE mcp_resource_subscriptions SET lease_until=now()+interval '15 seconds' WHERE session_id=$1 AND uri=$2 AND owner=$3 AND lease_until>now() AND expires_at>now()`, sid, uri, owner)
			cancel()
			if err != nil || tag.RowsAffected() != 1 {
				release(false)
				return
			}
		}
	}
}

func subscriptionOwnerAllowed(owner Owner, vs config.VirtualServer, snap *config.Snapshot) bool {
	switch owner.Mode {
	case "anonymous":
		return !vs.AuthRequired
	case "agent-key":
		for _, key := range snap.AgentKeys {
			if key.ID == owner.CredentialID && key.Servers[vs.Slug] && (key.ExpiresAt == nil || time.Now().Before(*key.ExpiresAt)) {
				return true
			}
		}
	case "oauth":
		for _, binding := range snap.Bindings {
			if binding.ID == owner.CredentialID && binding.Servers[vs.Slug] {
				return true
			}
		}
	}
	return false
}
