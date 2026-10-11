// audit-exporter runs independently of request handling, with its own DB pool.
package main

import (
	"context"
	"github.com/bishoku/vmcp/core/internal/audit"
	"github.com/jackc/pgx/v5/pgxpool"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := pgxpool.ParseConfig(os.Getenv("VMCP_AUDIT_EXPORT_DATABASE_URL"))
	if err != nil {
		log.Error("invalid exporter database configuration")
		os.Exit(1)
	}
	cfg.MaxConns = 2
	cfg.MinConns = 0
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		log.Error("exporter database unavailable")
		os.Exit(1)
	}
	defer pool.Close()
	exporter, err := audit.NewExporter(pool, os.Getenv("VMCP_AUDIT_EXPORT_CONSUMER"), os.Getenv("VMCP_AUDIT_EXPORT_URL"))
	if err != nil {
		log.Error("invalid exporter configuration")
		os.Exit(1)
	}
	if err := exporter.Register(ctx); err != nil {
		log.Error("exporter registration failed")
		os.Exit(1)
	}
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	stats := time.NewTicker(time.Minute)
	defer stats.Stop()
	for {
		select {
		case <-ctx.Done():
			return
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
