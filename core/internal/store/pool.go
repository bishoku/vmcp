package store

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"strconv"
	"time"
)

func poolConfig(databaseURL string) (*pgxpool.Config, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, err
	}
	for _, v := range []struct {
		name     string
		target   *int32
		fallback int
	}{
		{"VMCP_DB_MAX_CONNS", &cfg.MaxConns, 64}, {"VMCP_DB_MIN_CONNS", &cfg.MinConns, 4},
	} {
		n := v.fallback
		if raw := os.Getenv(v.name); raw != "" {
			n, err = strconv.Atoi(raw)
			if err != nil {
				return nil, fmt.Errorf("%s must be an integer", v.name)
			}
		}
		if n < 0 || n > 1000 {
			return nil, fmt.Errorf("%s must be between 0 and 1000", v.name)
		}
		*v.target = int32(n)
	}
	if os.Getenv("VMCP_DB_MIN_CONNS") == "" && cfg.MinConns > cfg.MaxConns {
		cfg.MinConns = cfg.MaxConns
	}
	if cfg.MaxConns < 2 || cfg.MinConns > cfg.MaxConns {
		return nil, fmt.Errorf("pool requires max >= 2 and min <= max")
	}
	timeout := 2 * time.Second
	if raw := os.Getenv("VMCP_DB_ACQUIRE_TIMEOUT"); raw != "" {
		timeout, err = time.ParseDuration(raw)
		if err != nil || timeout <= 0 || timeout > 30*time.Second {
			return nil, fmt.Errorf("VMCP_DB_ACQUIRE_TIMEOUT must be positive and at most 30s")
		}
	}
	cfg.ConnConfig.Tracer = &acquireDeadline{timeout: timeout}
	cfg.ConnConfig.ConnectTimeout = 5 * time.Second
	cfg.MaxConnLifetime = time.Hour
	cfg.MaxConnIdleTime = 5 * time.Minute
	return cfg, nil
}

type acquireDeadline struct{ timeout time.Duration }
type acquireCancelKey struct{}

func (a *acquireDeadline) TraceQueryStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryStartData) context.Context {
	return ctx
}
func (a *acquireDeadline) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}
func (a *acquireDeadline) TraceAcquireStart(ctx context.Context, _ *pgxpool.Pool, _ pgxpool.TraceAcquireStartData) context.Context {
	ctx, cancel := context.WithTimeout(ctx, a.timeout)
	return context.WithValue(ctx, acquireCancelKey{}, cancel)
}
func (a *acquireDeadline) TraceAcquireEnd(ctx context.Context, _ *pgxpool.Pool, _ pgxpool.TraceAcquireEndData) {
	if cancel, ok := ctx.Value(acquireCancelKey{}).(context.CancelFunc); ok {
		cancel()
	}
}
