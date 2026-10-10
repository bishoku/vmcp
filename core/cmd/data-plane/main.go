package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/bishoku/vmcp/core/internal/agentauth"
	"github.com/bishoku/vmcp/core/internal/audit"
	"github.com/bishoku/vmcp/core/internal/config"
	"github.com/bishoku/vmcp/core/internal/health"
	"github.com/bishoku/vmcp/core/internal/ratelimit"
	"github.com/bishoku/vmcp/core/internal/resilience"
	"github.com/bishoku/vmcp/core/internal/secrets"
	"github.com/bishoku/vmcp/core/internal/snapshot"
	"github.com/bishoku/vmcp/core/internal/store"
	"github.com/bishoku/vmcp/core/internal/virtual"
)

const sessionIdleTTL = 30 * time.Minute

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("service", "data-plane")
	if err := run(log); err != nil {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dbURL, err := config.MustEnv("VMCP_DATABASE_URL")
	if err != nil {
		return err
	}
	masterKey, err := config.MustEnv("VMCP_MASTER_KEY")
	if err != nil {
		return err
	}
	box, err := secrets.NewBox(masterKey)
	if err != nil {
		return err
	}

	pool, err := store.Connect(ctx, dbURL)
	if err != nil {
		return err
	}
	auditor := audit.NewWriter(ctx, pool, log)
	// Cancel background goroutines (the NOTIFY listener holds a pool connection)
	// and let the audit writer flush before closing the pool, otherwise Close
	// blocks on error exits and buffered events are lost.
	defer func() {
		stop()
		<-auditor.Done()
		pool.Close()
	}()

	syncer := &snapshot.Syncer{Pool: pool, Box: box, Log: log}
	if err := syncer.Start(ctx); err != nil {
		return err
	}

	sessions := virtual.NewManager()
	sessions.Store = virtual.PGSessionStore{Pool: pool}
	go sessions.RunReaper(ctx, sessionIdleTTL)

	// Consecutive failures before an upstream is cut off, and how long it stays cut off. 0 disables.
	threshold, err := strconv.Atoi(config.Env("VMCP_BREAKER_THRESHOLD", "5"))
	if err != nil || threshold < 0 {
		return errors.New("VMCP_BREAKER_THRESHOLD must be a non-negative integer")
	}
	cooldown, err := time.ParseDuration(config.Env("VMCP_BREAKER_COOLDOWN", "30s"))
	if err != nil || cooldown <= 0 {
		return errors.New("VMCP_BREAKER_COOLDOWN must be a positive duration such as 30s")
	}
	breakers := resilience.NewRegistry(threshold, cooldown)

	// Rate limits are per node unless Redis is configured; Redis is never required.
	var limiter ratelimit.Limiter = ratelimit.NewLocal()
	if url := config.Env("VMCP_REDIS_URL", ""); url != "" {
		if limiter, err = ratelimit.NewRedis(url, log); err != nil {
			return fmt.Errorf("VMCP_REDIS_URL: %w", err)
		}
		log.Info("rate limits are shared through Redis")
	} else {
		log.Info("rate limits are counted per node (set VMCP_REDIS_URL to share them)")
	}

	interval, err := time.ParseDuration(config.Env("VMCP_HEALTH_INTERVAL", "30s"))
	if err != nil || interval < 0 {
		return errors.New("VMCP_HEALTH_INTERVAL must be a duration such as 30s (0 disables)")
	}
	node := config.Env("VMCP_NODE_ID", "")
	if node == "" {
		node, _ = os.Hostname()
	}
	go (&health.Checker{Pool: pool, Snap: syncer, Breakers: breakers, Node: node, Interval: interval, Log: log}).Run(ctx)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, _ *http.Request) {
		if !syncer.Ready() {
			http.Error(w, "snapshot not loaded", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("ok"))
	})
	trusted, err := config.ParsePrefixes(config.Env("VMCP_TRUSTED_PROXIES", ""))
	if err != nil {
		log.Error("invalid VMCP_TRUSTED_PROXIES", "err", err)
		os.Exit(1)
	}
	var jwt *agentauth.Verifier
	if issuer := config.Env("VMCP_AGENT_OIDC_ISSUER", ""); issuer != "" {
		var algs []string
		for _, a := range strings.Split(config.Env("VMCP_AGENT_OIDC_ALGS", ""), ",") {
			if a = strings.TrimSpace(a); a != "" {
				algs = append(algs, a)
			}
		}
		jwt, err = agentauth.NewVerifier(ctx, agentauth.Config{
			Issuer: issuer, DiscoveryURL: config.Env("VMCP_AGENT_OIDC_DISCOVERY_URL", ""),
			Audience: config.Env("VMCP_AGENT_OIDC_AUDIENCE", ""), Algs: algs,
		})
		if err != nil {
			log.Error("agent oauth", "err", err)
			os.Exit(1)
		}
		log.Info("accepting access tokens from an identity provider", "issuer", issuer)
	}
	(&virtual.Handler{Snap: syncer, Sessions: sessions, Log: log, TrustedProxies: trusted, Audit: auditor, Breakers: breakers, Limiter: limiter,
		JWT: jwt, PublicURL: config.Env("VMCP_PUBLIC_URL", "")}).Register(mux)

	srv := &http.Server{
		Addr:              config.Env("VMCP_DP_ADDR", ":8080"),
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	log.Info("stopped")
	return nil
}
