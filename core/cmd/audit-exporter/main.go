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
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := auditexport.Run(ctx, log, func() (audit.Publisher, error) { return audit.NewHTTPPublisher(os.Getenv("VMCP_AUDIT_EXPORT_URL")) }); err != nil {
		log.Error("audit exporter stopped", "error", err)
		os.Exit(1)
	}
}
