package store

import (
	"context"
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
	t.Setenv("VMCP_DB_ACQUIRE_TIMEOUT", "25ms")
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
	start := time.Now()
	if _, err := pool.Acquire(context.Background()); err == nil {
		t.Fatal("saturated pool admitted another connection")
	}
	if time.Since(start) > time.Second {
		t.Fatal("acquisition exceeded budget")
	}
}
