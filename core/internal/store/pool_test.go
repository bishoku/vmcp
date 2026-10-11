package store

import (
	"context"
	"errors"
	"github.com/bishoku/vmcp/core/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
	"testing"
	"time"
)

func TestPoolConfigurationAndAcquisitionDeadline(t *testing.T) {
	t.Setenv("VMCP_DB_MAX_CONNS", "4")
	t.Setenv("VMCP_DB_MIN_CONNS", "2")
	t.Setenv("VMCP_DB_ACQUIRE_TIMEOUT", "10ms")
	cfg, err := poolConfig("postgres://localhost/test")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MaxConns != 4 || cfg.MinConns != 2 {
		t.Fatal("pool settings ignored")
	}
	tracer := cfg.ConnConfig.Tracer.(*acquireDeadline)
	ctx := tracer.TraceAcquireStart(context.Background(), nil, pgxpool.TraceAcquireStartData{})
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("acquisition timeout missing")
	}
	tracer.TraceAcquireEnd(ctx, nil, pgxpool.TraceAcquireEndData{})
	t.Setenv("VMCP_DB_MIN_CONNS", "5")
	if _, err := poolConfig("postgres://localhost/test"); err == nil {
		t.Fatal("invalid pool accepted")
	}
}

func TestSaturatedPoolFailsWithinAcquisitionBudget(t *testing.T) {
	t.Setenv("VMCP_DB_MAX_CONNS", "2")
	t.Setenv("VMCP_DB_MIN_CONNS", "0")
	t.Setenv("VMCP_DB_ACQUIRE_TIMEOUT", "5s")
	pool, err := Connect(context.Background(), testdb.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	a, err := pool.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer a.Release()
	b, err := pool.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer b.Release()
	// Allow connection setup to finish before testing the saturated pool's budget.
	pool.Config().ConnConfig.Tracer.(*acquireDeadline).timeout = 25 * time.Millisecond
	start := time.Now()
	if conn, err := pool.Acquire(context.Background()); !errors.Is(err, context.DeadlineExceeded) {
		if conn != nil {
			conn.Release()
		}
		t.Fatalf("saturated pool: expected acquisition deadline, got %v", err)
	}
	if time.Since(start) > time.Second {
		t.Fatal("acquisition exceeded budget")
	}
}

func TestViewKeepsExportReadsOnOneSnapshot(t *testing.T) {
	url := testdb.New(t)
	ctx := context.Background()
	if err := Migrate(ctx, url); err != nil {
		t.Fatal(err)
	}
	pool, err := Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	s := &Store{Pool: pool}
	if err := s.View(ctx, func(tx *Tx) error {
		first, err := tx.Revision(ctx)
		if err != nil {
			return err
		}
		if _, err := pool.Exec(ctx, `UPDATE config_revision SET revision=revision+1`); err != nil {
			return err
		}
		second, err := tx.Revision(ctx)
		if first != second {
			t.Fatal("export mixed database snapshots")
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}
