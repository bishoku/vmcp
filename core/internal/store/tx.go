package store

import (
	"context"
	"errors"
	"maps"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/bishoku/vmcp/core/internal/apierror"
	"github.com/bishoku/vmcp/core/internal/audit"
	"github.com/bishoku/vmcp/core/internal/secrets"
)

// tenantSQL scopes configuration to the internal row in each dedicated database.
// CheckTenant enforces the deployment identity; headers never select SQL tenancy.
const tenantSQL = `(SELECT id FROM tenants WHERE slug = 'default')`

// dbtx is satisfied by both *pgxpool.Pool and pgx.Tx.
type dbtx interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Store is the control plane's data access layer.
type Store struct {
	WireMockURL string
	Pool        *pgxpool.Pool
	Box         *secrets.Box
}

// Tx runs queries either inside a transaction (Do) or directly on the pool (Reader).
type Tx struct {
	// Optional fact staged by background discovery only when its catalog changes.
	mutation    *audit.Event
	wireMockURL string
	q           dbtx
	box         *secrets.Box
	targets     map[string]bool
}

// Reader returns a handle for read-only queries outside a transaction.
func (s *Store) Reader() *Tx { return &Tx{q: s.Pool, box: s.Box, wireMockURL: s.WireMockURL} }

// Do runs fn in a transaction. Any error rolls it back.
func (s *Store) Do(ctx context.Context, fn func(*Tx) error) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after Commit
	handle := &Tx{q: tx, box: s.Box, wireMockURL: s.WireMockURL, targets: map[string]bool{}}
	// Consistent lock order prevents apply revision checks and row mutations from deadlocking.
	if _, err := handle.LockRevision(ctx); err != nil {
		return err
	}
	if err := fn(handle); err != nil {
		return err
	}
	e, ok := audit.Transaction(ctx)
	if handle.mutation != nil {
		e, ok = *handle.mutation, true
	}
	if ok {
		// An audit failure rolls the entire configuration transaction back.
		if e.Mode != "relax" {
			if _, err := tx.Exec(ctx, `SET LOCAL synchronous_commit=on`); err != nil {
				return err
			}
		}
		e.Time = time.Now().UTC()
		e.Stage = "mutation"
		e.Status = "ok"
		e.EventID = ""
		e.Direction = ""
		e.RawPayload = nil
		e.Ciphertext = nil
		e.PayloadState = "disabled"
		e.PayloadOriginalBytes = 0
		e.PayloadStoredBytes = 0
		e.PayloadRedactions = 0
		if err := tx.QueryRow(ctx, `SELECT revision FROM config_revision`).Scan(&e.Revision); err != nil {
			return err
		}
		targets := slices.Sorted(maps.Keys(handle.targets))
		if len(targets) == 0 {
			targets = []string{e.Target}
		}
		var deferredEvents []audit.Event
		for _, target := range targets {
			e.Target = target
			if e.Mode == "relax" {
				deferredEvents = append(deferredEvents, e)
				continue
			}
			if err := audit.Insert(ctx, tx, e); err != nil {
				audit.ReportTransactionFailure(ctx, err)
				return apierror.New(503, "audit_unavailable", "Audit unavailable", "configuration transaction rolled back because audit persistence failed")
			}
		}
		if len(deferredEvents) > 0 {
			if err := tx.Commit(ctx); err != nil {
				return err
			}
			if runtime := audit.RuntimeFrom(ctx); runtime != nil {
				for _, event := range deferredEvents {
					_ = runtime.Mutation(context.WithoutCancel(ctx), event, nil)
				}
			}
			return nil
		}
	}
	return tx.Commit(ctx)
}

// Outcome describes what a write did.
type Outcome string

const (
	Created   Outcome = "create"
	Updated   Outcome = "update"
	Unchanged Outcome = "unchanged"
	Deleted   Outcome = "delete"
)

// mapWriteErr turns a unique violation (lost race on a slug) into a 409.
func mapWriteErr(err error, kind, slug string) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return apierror.AlreadyExists(kind, slug)
	}
	return err
}

// auditTarget records identifiers only, never writable values or decrypted secrets.
func (t *Tx) auditTarget(kind, slug string) {
	if t.targets != nil && ValidSlug(slug) {
		t.targets[kind+":"+slug] = true
	}
}

// View executes related reads against one repeatable, read-only snapshot.
func (s *Store) View(ctx context.Context, fn func(*Tx) error) error {
	tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := fn(&Tx{q: tx, box: s.Box, wireMockURL: s.WireMockURL}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (tx *Tx) Revision(ctx context.Context) (int64, error) {
	var revision int64
	err := tx.q.QueryRow(ctx, "SELECT revision FROM config_revision").Scan(&revision)
	return revision, err
}

func (tx *Tx) LockRevision(ctx context.Context) (int64, error) {
	var revision int64
	err := tx.q.QueryRow(ctx, "SELECT revision FROM config_revision FOR UPDATE").Scan(&revision)
	return revision, err
}
