// stdio-worker runs on a separate host with a dedicated rootless Docker daemon.
package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"github.com/bishoku/vmcp/core/internal/stdioworker"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	var profiles map[string]stdioworker.Profile
	if json.Unmarshal([]byte(os.Getenv("VMCP_STDIO_PROFILES")), &profiles) != nil {
		log.Error("invalid worker profile configuration")
		os.Exit(1)
	}
	worker, err := stdioworker.New(profiles)
	if err != nil {
		log.Error("invalid worker allowlist")
		os.Exit(1)
	}
	defer worker.Close()
	ca, err := os.ReadFile(os.Getenv("VMCP_STDIO_CLIENT_CA"))
	roots := x509.NewCertPool()
	if err != nil || !roots.AppendCertsFromPEM(ca) {
		log.Error("worker requires a client CA")
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		tick := time.NewTicker(time.Minute)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-tick.C:
				worker.Sweep(now)
			}
		}
	}()
	address := os.Getenv("VMCP_STDIO_ADDR")
	if address == "" {
		address = ":8443"
	}
	server := &http.Server{Addr: address, Handler: worker, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 75 * time.Second, MaxHeaderBytes: 16 << 10, TLSConfig: &tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: roots}}
	go func() {
		<-ctx.Done()
		worker.Close()
		bounded, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(bounded)
	}()
	if err := server.ListenAndServeTLS(os.Getenv("VMCP_STDIO_CERT"), os.Getenv("VMCP_STDIO_KEY")); err != nil && err != http.ErrServerClosed {
		log.Error("worker listener failed")
		os.Exit(1)
	}
}
