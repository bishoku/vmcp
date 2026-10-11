// Package delivery persists session-scoped MCP routing and replay independently of nodes.
package delivery

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/bishoku/vmcp/core/internal/audit"
	"sync"
	"time"

	"github.com/bishoku/vmcp/core/internal/secrets"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrCapacity = errors.New("MCP delivery capacity or ownership unavailable")

type Store struct {
	Pool        *pgxpool.Pool
	Box         *secrets.Box
	ListenerURL string
	watchMu     sync.Mutex
	watchers    map[string]map[string]chan struct{}
}
type Operation struct{ ID, Fence, Stream, Session string }
type Event struct {
	Seq      int64
	ID       string
	Payload  []byte
	Terminal bool
}

func UUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	b[6] = (b[6] & 15) | 64
	b[8] = (b[8] & 63) | 128
	h := hex.EncodeToString(b[:])
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}

func (s *Store) Begin(ctx context.Context, session, request, server string, principal []byte) (Operation, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	op := Operation{ID: UUID(), Fence: UUID(), Stream: UUID(), Session: session}
	if len(request) > 1024 {
		return op, ErrCapacity
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return op, err
	}
	defer tx.Rollback(ctx)
	// Admission locks serialize capacity checks across replicas, not just within one process.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(19840923)`); err != nil {
		return op, err
	}
	if _, err := tx.Exec(ctx, `UPDATE mcp_operations SET state='abandoned' WHERE state='running' AND lease_until<now()`); err != nil {
		return op, err
	}
	var all, perPrincipal, perServer int
	if err := tx.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE principal_hash=$1),count(*) FILTER(WHERE virtual_server=$2) FROM mcp_operations WHERE state='running'`, principal, server).Scan(&all, &perPrincipal, &perServer); err != nil {
		return op, err
	}
	if all >= 1024 || perPrincipal >= 16 || perServer >= 128 {
		return op, ErrCapacity
	}
	_, err = tx.Exec(ctx, `INSERT INTO mcp_operations(id,session_id,client_request_id,principal_hash,virtual_server,fence,lease_until) SELECT $1,$2,$3,$4,$5,$6,now()+interval '15 seconds' FROM sessions WHERE id=$2 AND terminated_at IS NULL`, op.ID, session, request, principal, server, op.Fence)
	if err != nil {
		return op, ErrCapacity
	}
	if _, err = tx.Exec(ctx, `INSERT INTO mcp_streams(id,session_id,operation_id) VALUES($1,$2,$3)`, op.Stream, session, op.ID); err != nil {
		return op, err
	}
	return op, tx.Commit(ctx)
}
func (s *Store) Renew(ctx context.Context, op Operation) (bool, error) {
	var cancelled bool
	err := s.Pool.QueryRow(ctx, `UPDATE mcp_operations SET lease_until=now()+interval '15 seconds' WHERE id=$1 AND fence=$2 AND state='running' AND lease_until>now() RETURNING cancelled`, op.ID, op.Fence).Scan(&cancelled)
	return cancelled, err
}
func (s *Store) Cancel(ctx context.Context, session, request string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE mcp_operations SET cancelled=true WHERE session_id=$1 AND client_request_id=$2 AND state='running'`, session, request)
	return err
}
func (s *Store) Append(ctx context.Context, op Operation, payload []byte, terminal bool) error {
	sealed, err := s.Box.Seal(payload)
	if err != nil {
		return err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var count, totalBytes int
	// Lock/fence the operation before accepting an event from a possibly superseded worker.
	if err := tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM mcp_events WHERE stream_id=$3),(SELECT COALESCE(sum(octet_length(payload)),0) FROM mcp_events WHERE stream_id=$3) FROM mcp_operations WHERE id=$1 AND fence=$2 AND state='running' AND lease_until>now() FOR UPDATE`, op.ID, op.Fence, op.Stream).Scan(&count, &totalBytes); err != nil {
		return err
	}
	if (terminal && count >= 256) || (!terminal && count >= 255) || (!terminal && totalBytes+len(sealed) > 2<<20) || totalBytes+len(sealed) > 12<<20 {
		return ErrCapacity
	}
	if _, err := tx.Exec(ctx, `INSERT INTO mcp_events(id,stream_id,payload,terminal) VALUES($1,$2,$3,$4)`, UUID(), op.Stream, sealed, terminal); err != nil {
		return err
	}
	if terminal {
		if _, err := tx.Exec(ctx, `UPDATE mcp_operations SET state='done' WHERE id=$1 AND fence=$2`, op.ID, op.Fence); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `SELECT pg_notify('vmcp_delivery',$1)`, op.Session); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (s *Store) GetStream(ctx context.Context, session string) (string, error) {
	var id string
	err := s.Pool.QueryRow(ctx, `INSERT INTO mcp_streams(id,session_id) VALUES($1,$2) ON CONFLICT(session_id) WHERE operation_id IS NULL AND NOT closed DO UPDATE SET expires_at=now()+interval '15 minutes' RETURNING id::text`, UUID(), session).Scan(&id)
	return id, err
}
func (s *Store) Resume(ctx context.Context, session, event string) (stream string, seq int64, err error) {
	err = s.Pool.QueryRow(ctx, `SELECT e.stream_id::text,e.seq FROM mcp_events e JOIN mcp_streams s ON s.id=e.stream_id WHERE e.id=$1 AND s.session_id=$2 AND s.expires_at>now()`, event, session).Scan(&stream, &seq)
	return
}
func (s *Store) Claim(ctx context.Context, session, stream, owner string) error {
	tag, err := s.Pool.Exec(ctx, `UPDATE mcp_streams SET owner=$3,lease_until=now()+interval '15 seconds' WHERE id=$1 AND session_id=$2 AND expires_at>now() AND (owner=$3 OR lease_until IS NULL OR lease_until<now())`, stream, session, owner)
	if err == nil && tag.RowsAffected() != 1 {
		return ErrCapacity
	}
	return err
}
func (s *Store) Release(ctx context.Context, stream, owner string) {
	_, _ = s.Pool.Exec(ctx, `UPDATE mcp_streams SET owner=NULL,lease_until=NULL WHERE id=$1 AND owner=$2`, stream, owner)
}
func (s *Store) Events(ctx context.Context, session, stream, owner string, after int64) ([]Event, error) {
	rows, err := s.Pool.Query(ctx, `SELECT e.seq,e.id::text,e.payload,e.terminal FROM mcp_events e JOIN mcp_streams s ON s.id=e.stream_id WHERE s.id=$1 AND s.session_id=$2 AND s.owner=$3 AND s.lease_until>now() AND s.expires_at>now() AND e.seq>$4 ORDER BY e.seq LIMIT 32`, stream, session, owner, after)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var events []Event
	for rows.Next() {
		var e Event
		var sealed []byte
		if err := rows.Scan(&e.Seq, &e.ID, &sealed, &e.Terminal); err != nil {
			return nil, err
		}
		e.Payload, err = s.Box.Open(sealed)
		if err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}
func (s *Store) Reply(ctx context.Context, session, id string, response []byte) error {
	sealed, err := s.Box.Seal(response)
	if err != nil {
		return err
	}
	tag, err := s.Pool.Exec(ctx, `UPDATE mcp_pending_requests p SET response=$3 FROM mcp_operations o WHERE p.id=$1 AND p.session_id=$2 AND p.response IS NULL AND p.expires_at>now() AND o.id=p.operation_id AND o.state='running' AND o.lease_until>now()`, id, session, sealed)
	if err == nil && tag.RowsAffected() != 1 {
		return ErrCapacity
	}
	return err
}
func (s *Store) Request(ctx context.Context, op Operation, id string) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var count int
	if err := tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM mcp_pending_requests WHERE operation_id=$1) FROM mcp_operations WHERE id=$1 AND fence=$2 AND state='running' AND lease_until>now() FOR UPDATE`, op.ID, op.Fence).Scan(&count); err != nil {
		return err
	}
	if count >= 32 {
		return ErrCapacity
	}
	if _, err := tx.Exec(ctx, `INSERT INTO mcp_pending_requests(id,operation_id,session_id,expires_at) VALUES($1,$2,$3,now()+interval '60 seconds')`, id, op.ID, op.Session); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) WaitReply(ctx context.Context, op Operation, id string) ([]byte, error) {
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_, _ = s.Pool.Exec(cleanup, `DELETE FROM mcp_pending_requests WHERE id=$1 AND operation_id=$2`, id, op.ID)
	}()
	for {
		var sealed []byte
		err := s.Pool.QueryRow(ctx, `SELECT response FROM mcp_pending_requests WHERE id=$1 AND operation_id=$2 AND expires_at>now()`, id, op.ID).Scan(&sealed)
		if err != nil {
			return nil, err
		}
		if len(sealed) > 0 {
			return s.Box.Open(sealed)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-tick.C:
		}
	}
}
func (s *Store) Purge(ctx context.Context) error {
	_, err := s.Pool.Exec(ctx, `UPDATE mcp_operations SET state='abandoned' WHERE id IN (SELECT id FROM mcp_operations WHERE state='running' AND lease_until<now() LIMIT 100); DELETE FROM mcp_streams WHERE id IN (SELECT id FROM mcp_streams WHERE expires_at<now() LIMIT 100); DELETE FROM mcp_operations WHERE id IN (SELECT id FROM mcp_operations WHERE state<>'running' AND created_at<now()-interval '15 minutes' LIMIT 100)`)
	return err
}

// PublishListChanged batches session-local notifications and deduplicates concurrent publishers.
func (s *Store) PublishListChanged(ctx context.Context, revision int64) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `SELECT id::text,session_id FROM mcp_streams WHERE operation_id IS NULL AND NOT closed AND expires_at>now() AND last_revision<$1 ORDER BY id LIMIT 100 FOR UPDATE SKIP LOCKED`, revision)
	if err != nil {
		return err
	}
	var ids []string
	sessions := map[string]string{}
	for rows.Next() {
		var id, session string
		if err := rows.Scan(&id, &session); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
		sessions[id] = session
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, id := range ids {
		var count int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM mcp_events WHERE stream_id=$1`, id).Scan(&count); err != nil {
			return err
		}
		if count >= 256 {
			if _, err := tx.Exec(ctx, `UPDATE mcp_streams SET closed=true WHERE id=$1`, id); err != nil {
				return err
			}
			continue
		}
		methods := []string{"notifications/tools/list_changed"}
		var resource, prompt bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM virtual_catalog vc JOIN virtual_servers v ON v.id=vc.virtual_server_id JOIN sessions s ON s.virtual_server=v.slug WHERE s.id=$1 AND vc.kind IN ('resource','template')),EXISTS(SELECT 1 FROM virtual_catalog vc JOIN virtual_servers v ON v.id=vc.virtual_server_id JOIN sessions s ON s.virtual_server=v.slug WHERE s.id=$1 AND vc.kind='prompt')`, sessions[id]).Scan(&resource, &prompt); err != nil {
			return err
		}
		if resource {
			methods = append(methods, "notifications/resources/list_changed")
		}
		if prompt {
			methods = append(methods, "notifications/prompts/list_changed")
		}
		if count+len(methods) > 256 {
			if _, err := tx.Exec(ctx, `UPDATE mcp_streams SET closed=true WHERE id=$1`, id); err != nil {
				return err
			}
			continue
		}
		for _, method := range methods {
			if err := audit.Insert(ctx, tx, audit.Event{Time: time.Now().UTC(), ActorType: "system", Action: "mcp.list_changed", Method: method, Target: sessions[id], Status: "ok", Stage: "event", Revision: revision}); err != nil {
				return err
			}
			raw, _ := json.Marshal(map[string]string{"jsonrpc": "2.0", "method": method})
			sealed, err := s.Box.Seal(raw)
			if err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO mcp_events(id,stream_id,payload) VALUES($1,$2,$3)`, UUID(), id, sealed); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE mcp_streams SET last_revision=$2 WHERE id=$1`, id, revision); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `SELECT pg_notify('vmcp_delivery',$1)`, sessions[id]); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
func (s *Store) Run(ctx context.Context, revision func() int64) {
	go s.listen(ctx)
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			bounded, cancel := context.WithTimeout(ctx, time.Second)
			_ = s.PublishListChanged(bounded, revision())
			_ = s.Purge(bounded)
			cancel()
		}
	}
}

func (s *Store) CancelPending(ctx context.Context, op Operation, id string, response []byte) error {
	sealed, err := s.Box.Seal(response)
	if err != nil {
		return err
	}
	_, err = s.Pool.Exec(ctx, `UPDATE mcp_pending_requests SET response=$3 WHERE id=$1 AND operation_id=$2 AND response IS NULL`, id, op.ID, sealed)
	return err
}
