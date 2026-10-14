package main

import (
	"context"
	"encoding/json"
	"github.com/bishoku/vmcp/core/internal/tenancy"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	var directory []tenancy.Tenant
	if json.Unmarshal([]byte(os.Getenv("VMCP_TENANT_DIRECTORY")), &directory) != nil {
		log.Error("invalid tenant directory")
		os.Exit(1)
	}
	router, err := tenancy.New(directory)
	if err != nil {
		log.Error("invalid tenant boundaries")
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	address := os.Getenv("VMCP_TENANT_ADDR")
	if address == "" {
		address = ":8443"
	}
	server := &http.Server{Addr: address, Handler: router, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, IdleTimeout: 120 * time.Second, MaxHeaderBytes: 16 << 10}
	go func() {
		<-ctx.Done()
		bounded, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		_ = server.Shutdown(bounded)
	}()
	if err := server.ListenAndServeTLS(os.Getenv("VMCP_TENANT_CERT"), os.Getenv("VMCP_TENANT_KEY")); err != nil && err != http.ErrServerClosed {
		log.Error("tenant router listener failed")
		os.Exit(1)
	}
}
