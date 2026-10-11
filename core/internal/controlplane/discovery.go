package controlplane

import (
	"context"
	"errors"
	"time"

	"github.com/bishoku/vmcp/core/internal/audit"
	"github.com/bishoku/vmcp/core/internal/delivery"
	"github.com/bishoku/vmcp/core/internal/store"
	"github.com/jackc/pgx/v5"
)

// RunDiscovery reconciles queued list changes and polls enabled catalogs as a fallback.
// Each control-plane instance has two workers; SQL leases prevent duplicate ownership.
func (s *Server) RunDiscovery(ctx context.Context) {
	for i := 0; i < 2; i++ {
		go s.discoveryWorker(ctx)
	}
	tick := time.NewTicker(30 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
			_, _ = s.Store.Pool.Exec(bounded, `INSERT INTO mcp_discovery_requests(upstream_id) SELECT id FROM upstreams WHERE enabled AND (last_refresh_at IS NULL OR last_refresh_at<now()-interval '30 seconds') ON CONFLICT(upstream_id) DO NOTHING`)
			cancel()
		}
	}
}
func (s *Server) discoveryWorker(ctx context.Context) {
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			s.discoverOne(ctx)
		}
	}
}
func (s *Server) discoverOne(parent context.Context) {
	ctx, cancel := context.WithTimeout(parent, 65*time.Second)
	defer cancel()
	owner := delivery.UUID()
	var id, slug string
	var requested time.Time
	err := s.Store.Pool.QueryRow(ctx, `UPDATE mcp_discovery_requests q SET lease_owner=$1,lease_until=now()+interval '90 seconds' FROM upstreams u WHERE q.upstream_id=u.id AND q.upstream_id=(SELECT upstream_id FROM mcp_discovery_requests WHERE lease_until IS NULL OR lease_until<now() ORDER BY requested_at LIMIT 1 FOR UPDATE SKIP LOCKED) RETURNING q.upstream_id::text,u.slug,q.requested_at`, owner).Scan(&id, &slug, &requested)
	if errors.Is(err, pgx.ErrNoRows) || err != nil {
		return
	}
	revision, err := store.Revision(ctx, s.Store.Pool)
	if err != nil {
		return
	}
	recorder := audit.NewDurable(s.Store.Pool, s.Log)
	event, err := recorder.Admit(ctx, audit.Event{Time: time.Now().UTC(), ActorType: "system", ActorID: "discovery", Action: "upstream.discovery", Method: "upstream.discovery", Target: slug, Revision: revision})
	if err != nil {
		return
	}
	conn, err := s.Store.Reader().UpstreamConnInfo(ctx, slug)
	if err == nil {
		probeCtx, probeCancel := context.WithTimeout(ctx, 55*time.Second)
		result, probeErr := probeAny(probeCtx, append([]string{conn.URL}, conn.FallbackURLs...), conn.Transport, conn.Headers, conn.TLS, conn.OAuth)
		probeCancel()
		err = probeErr
		if err == nil {
			err = s.Store.Do(audit.WithTransaction(ctx, event), func(tx *store.Tx) error {
				current, err := tx.Revision(ctx)
				if err != nil {
					return err
				}
				if current != revision {
					return errors.New("configuration changed during discovery")
				}
				if _, err := tx.ReplaceTools(ctx, slug, result.Tools, time.Now().UTC()); err != nil {
					return err
				}
				return tx.SetRefreshStatus(ctx, slug, time.Now().UTC(), nil)
			})
		}
	}
	completion, done := context.WithTimeout(context.WithoutCancel(ctx), audit.WriteTimeout)
	defer done()
	event.Status = "ok"
	if err != nil {
		event.Status = "error"
		event.Reason = "catalog reconciliation failed"
	}
	if recorder.Complete(completion, event) != nil {
		return
	}
	if err == nil {
		_, _ = s.Store.Pool.Exec(completion, `DELETE FROM mcp_discovery_requests WHERE upstream_id=$1 AND lease_owner=$2 AND requested_at=$3`, id, owner, requested)
		// A notification arriving during discovery must get a fresh reconciliation.
		_, _ = s.Store.Pool.Exec(completion, `UPDATE mcp_discovery_requests SET lease_owner=NULL,lease_until=NULL WHERE upstream_id=$1 AND lease_owner=$2`, id, owner)
	} else {
		_, _ = s.Store.Pool.Exec(completion, `UPDATE mcp_discovery_requests SET lease_owner=NULL,lease_until=now()+interval '5 seconds' WHERE upstream_id=$1 AND lease_owner=$2`, id, owner)
	}
}
