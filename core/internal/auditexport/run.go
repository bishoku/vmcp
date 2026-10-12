// Package auditexport runs independent, transport-specific audit workers.
package auditexport

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"time"

	"github.com/bishoku/vmcp/core/internal/audit"
	"github.com/bishoku/vmcp/core/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

func Run(ctx context.Context, log *slog.Logger, factory func() (audit.Publisher, error)) error {
	if os.Getenv("VMCP_AUDIT_EXPORT_DATABASE_URL") == "" {
		return errors.New("exporter database URL is required")
	}
	cfg, err := pgxpool.ParseConfig(os.Getenv("VMCP_AUDIT_EXPORT_DATABASE_URL"))
	if err != nil {
		return errors.New("invalid exporter database configuration")
	}
	cfg.MaxConns = 2
	cfg.MinConns = 0
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return errors.New("exporter database unavailable")
	}
	defer pool.Close()
	if err := store.CheckTenant(ctx, pool); err != nil {
		return errors.New("exporter tenant database identity mismatch")
	}
	publisher, err := factory()
	if err != nil {
		return errors.New("invalid audit publisher configuration")
	}
	defer publisher.Close()
	exporter, err := audit.NewPublishingExporter(pool, os.Getenv("VMCP_AUDIT_EXPORT_CONSUMER"), publisher)
	if err != nil {
		return err
	}
	if err := exporter.Register(ctx); err != nil {
		return errors.New("exporter registration failed")
	}
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	stats := time.NewTicker(time.Minute)
	defer stats.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-stats.C:
			bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
			counts, err := exporter.Stats(bounded)
			cancel()
			if err == nil {
				log.Info("audit delivery counts", "counts", string(counts))
			}
		case <-tick.C:
			for i := 0; i < 100; i++ {
				bounded, cancel := context.WithTimeout(ctx, 10*time.Second)
				more, err := exporter.Step(bounded)
				cancel()
				if err != nil {
					log.Error("audit export attempt failed")
					break
				}
				if !more {
					break
				}
			}
		}
	}
}
