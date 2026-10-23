package delivery

import (
	"context"
	"encoding/json"
	"github.com/bishoku/vmcp/core/internal/audit"
	"time"
)

// ResourceUpdated fences the physical watcher and atomically appends one hint
// to the session's GET stream. No resource body or original URI is persisted.
func (s *Store) ResourceUpdated(ctx context.Context, session, uri, owner string) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var revision int64
	if err := tx.QueryRow(ctx, `SELECT revision FROM mcp_resource_subscriptions WHERE session_id=$1 AND uri=$2 AND owner=$3 AND lease_until>now() AND expires_at>now() AND NOT failed AND EXISTS(SELECT 1 FROM sessions WHERE id=$1 AND terminated_at IS NULL AND initialized) FOR UPDATE`, session, uri, owner).Scan(&revision); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO mcp_streams(id,session_id) VALUES($1,$2) ON CONFLICT(session_id) WHERE operation_id IS NULL AND NOT closed DO NOTHING`, UUID(), session); err != nil {
		return err
	}
	var stream string
	if err := tx.QueryRow(ctx, `SELECT id::text FROM mcp_streams WHERE session_id=$1 AND operation_id IS NULL AND NOT closed AND expires_at>now() FOR UPDATE`, session).Scan(&stream); err != nil {
		return err
	}
	var count int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM mcp_events WHERE stream_id=$1`, stream).Scan(&count); err != nil {
		return err
	}
	if overflow := (count + 1) - 256; overflow > 0 {
		if _, err := tx.Exec(ctx, `DELETE FROM mcp_events WHERE seq IN (SELECT seq FROM mcp_events WHERE stream_id=$1 ORDER BY seq ASC LIMIT $2)`, stream, overflow); err != nil {
			return err
		}
	}
	raw, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "method": "notifications/resources/updated", "params": map[string]string{"uri": uri}})
	eventID := UUID()
	sealed, err := s.Box.SealBound(raw, eventID, "mcp.event")
	if err != nil {
		return err
	}
	if err := audit.Insert(ctx, tx, audit.Event{Time: time.Now().UTC(), ActorType: "system", Action: "mcp.resource_updated", Method: "notifications/resources/updated", Target: session, Status: "ok", Stage: "event", Revision: revision}); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO mcp_events(id,stream_id,payload) VALUES($1,$2,$3)`, eventID, stream, sealed); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	s.NotifyLocal(session)
	return nil
}
