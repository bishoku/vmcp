// Package store contains Postgres access shared by both planes.
package store

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver
	"github.com/pressly/goose/v3"

	"github.com/bishoku/vmcp/core/migrations"
)

// Connect opens a pgx connection pool and verifies connectivity.
func Connect(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	cfg, err := poolConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("pool configuration: %w", err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("open pool: %w", err)
	}
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return pool, nil
}

var migrationMu sync.Mutex

// Migrate applies all pending migrations. Only the control plane should call it.
func Migrate(ctx context.Context, databaseURL string) error {
	migrationMu.Lock()
	defer migrationMu.Unlock()
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	// Session-level lock is held on a dedicated connection, so concurrent
	// replicas cannot interleave migration discovery and application.
	bounded, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	ctx = bounded
	lock, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer lock.Close()
	if _, err := lock.ExecContext(ctx, `SELECT pg_advisory_lock(784321901)`); err != nil {
		return err
	}
	defer func() {
		unlock, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = lock.ExecContext(unlock, `SELECT pg_advisory_unlock(784321901)`)
	}()
	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("set dialect: %w", err)
	}
	if err := goose.UpContext(ctx, db, "."); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}

// CheckSchema rejects startup against an unmigrated or older schema.
func CheckSchema(ctx context.Context, pool *pgxpool.Pool) error {
	entries, err := migrations.FS.ReadDir(".")
	if err != nil {
		return err
	}
	var required int64
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		version, err := strconv.ParseInt(strings.SplitN(entry.Name(), "_", 2)[0], 10, 64)
		if err != nil {
			return err
		}
		required = max(required, version)
	}
	var actual int64
	bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.QueryRow(bounded, `SELECT COALESCE(max(version_id),0) FROM goose_db_version WHERE is_applied`).Scan(&actual); err != nil {
		return fmt.Errorf("database schema unavailable")
	}
	if actual < required {
		return fmt.Errorf("database migration required: have %d need %d", actual, required)
	}
	return nil
}
