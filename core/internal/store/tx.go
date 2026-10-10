package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/bishoku/vmcp/core/internal/apierror"
	"github.com/bishoku/vmcp/core/internal/secrets"
)

// tenantSQL scopes every query to the default tenant until tenancy is exposed.
const tenantSQL = `(SELECT id FROM tenants WHERE slug = 'default')`

// dbtx is satisfied by both *pgxpool.Pool and pgx.Tx.
type dbtx interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Store is the control plane's data access layer.
type Store struct {
	Pool *pgxpool.Pool
	Box  *secrets.Box
}

// Tx runs queries either inside a transaction (Do) or directly on the pool (Reader).
type Tx struct {
	q   dbtx
	box *secrets.Box
}

// Reader returns a handle for read-only queries outside a transaction.
func (s *Store) Reader() *Tx { return &Tx{q: s.Pool, box: s.Box} }

// Do runs fn in a transaction. Any error rolls it back.
func (s *Store) Do(ctx context.Context, fn func(*Tx) error) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after Commit
	if err := fn(&Tx{q: tx, box: s.Box}); err != nil {
		return err
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
