// Runs independently of the control and data planes.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/bishoku/vmcp/core/internal/audit"
	"github.com/bishoku/vmcp/core/internal/auditexport"
	"github.com/bishoku/vmcp/core/internal/telemetry"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	log, err := telemetry.NewLogger("audit-exporter")
	if err != nil {
		slog.Error("invalid logging configuration")
		os.Exit(1)
	}
	slog.SetDefault(log)
	if err := auditexport.Run(ctx, log, func() (audit.Publisher, error) { return audit.NewHTTPPublisher(os.Getenv("VMCP_AUDIT_EXPORT_URL")) }); err != nil {
		log.Error("audit exporter stopped", "error", err)
		os.Exit(1)
	}
}
