package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/bishoku/vmcp/core/internal/config"
	"github.com/bishoku/vmcp/core/internal/omni"
)

func main() {
	if err := run(); err != nil {
		slog.Error("omni stopped", "error", err)
		os.Exit(1)
	}
}
func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	backend, err := omni.NewBackend(config.Env("VMCP_OMNI_DATA_PLANE_URL", ""), config.Env("VMCP_OMNI_SECRET", ""))
	if err != nil {
		return err
	}
	insecureEmbeddingTLS, err := strconv.ParseBool(config.Env("VMCP_OMNI_EMBEDDING_INSECURE_SKIP_VERIFY", "false"))
	if err != nil {
		return errors.New("VMCP_OMNI_EMBEDDING_INSECURE_SKIP_VERIFY must be a boolean")
	}
	var embedder omni.Embedder
	if endpoint := config.Env("VMCP_OMNI_EMBEDDING_URL", ""); endpoint != "" {
		e, err := omni.NewHTTPEmbedder(endpoint, config.Env("VMCP_OMNI_EMBEDDING_MODEL", ""), config.Env("VMCP_OMNI_EMBEDDING_KEY", ""), insecureEmbeddingTLS)
		if err != nil {
			return err
		}
		embedder = e
	}
	interval, err := time.ParseDuration(config.Env("VMCP_OMNI_REFRESH_INTERVAL", "30s"))
	if err != nil || interval < time.Second {
		return errors.New("refresh interval must be at least 1s")
	}
	proxies, err := config.ParsePrefixes(config.Env("VMCP_TRUSTED_PROXIES", ""))
	if err != nil {
		return err
	}
	index := omni.NewIndex(embedder)
	go index.Run(ctx, backend, interval)
	limit, err := strconv.Atoi(config.Env("VMCP_OMNI_SEARCH_LIMIT", "120"))
	if err != nil || limit < 1 {
		return errors.New("VMCP_OMNI_SEARCH_LIMIT must be positive")
	}
	app := &omni.Server{SearchLimit: limit, Backend: backend, Index: index, TrustedProxies: proxies, PublicURL: config.Env("VMCP_OMNI_PUBLIC_URL", ""), Issuer: config.Env("VMCP_AGENT_OIDC_ISSUER", "")}
	server := &http.Server{Addr: config.Env("VMCP_LISTEN", ":8090"), Handler: app.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 75 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 32 << 10}
	go func() {
		<-ctx.Done()
		c, cancel := context.WithTimeout(context.Background(), 70*time.Second)
		defer cancel()
		_ = server.Shutdown(c)
	}()
	err = server.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
