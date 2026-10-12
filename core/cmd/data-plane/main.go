package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/bishoku/vmcp/core/internal/lifecycle"
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
	"github.com/bishoku/vmcp/core/internal/delivery"
	"github.com/bishoku/vmcp/core/internal/health"
	"github.com/bishoku/vmcp/core/internal/netpolicy"
	"github.com/bishoku/vmcp/core/internal/oauthtoken"
	"github.com/bishoku/vmcp/core/internal/ratelimit"
	"github.com/bishoku/vmcp/core/internal/resilience"
	"github.com/bishoku/vmcp/core/internal/secrets"
	"github.com/bishoku/vmcp/core/internal/snapshot"
	"github.com/bishoku/vmcp/core/internal/store"
	"github.com/bishoku/vmcp/core/internal/telemetry"
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
	if _, err := netpolicy.FromEnv(); err != nil {
		return err
	}
	if _, err := netpolicy.Origins(); err != nil {
		return err
	}
	signalCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	ctx, cancelBackground := context.WithCancel(context.Background())
	defer cancelBackground()
	auditCtx, cancelAudit := context.WithCancel(context.Background())
	defer cancelAudit()
	defer stop()

	dbURL, err := config.MustEnv("VMCP_DATABASE_URL")
	if err != nil {
		return err
	}
	masterKey, err := config.MustEnv("VMCP_MASTER_KEY")
	if err != nil {
		return err
	}
	box, err := secrets.FromEnv(masterKey)
	if err != nil {
		return err
	}

	pool, err := store.Connect(ctx, dbURL)
	if err != nil {
		return err
	}
	if err := store.CheckSchema(ctx, pool); err != nil {
		pool.Close()
		return err
	}
	auditor, err := audit.NewService(auditCtx, pool, log, config.Env("VMCP_AUDIT_MODE", "strict"))
	if err != nil {
		pool.Close()
		return err
	}
	// Cancel background goroutines (the NOTIFY listener holds a pool connection)
	// and let the audit writer flush before closing the pool, otherwise Close
	// blocks on error exits and buffered events are lost.
	defer func() {
		cancelBackground()
		cancelAudit()
		<-auditor.Done()
		pool.Close()
	}()

	drainTimeout, err := time.ParseDuration(config.Env("VMCP_DRAIN_TIMEOUT", "75s"))
	if err != nil || drainTimeout < 65*time.Second || drainTimeout > 2*time.Minute {
		return errors.New("VMCP_DRAIN_TIMEOUT must be between 65s and 2m")
	}
	maxStale, err := time.ParseDuration(config.Env("VMCP_AUTH_MAX_STALE", "30s"))
	if err != nil || maxStale <= 0 {
		return errors.New("VMCP_AUTH_MAX_STALE must be a positive duration")
	}
	sessions := virtual.NewManager()
	defer sessions.Close()
	sessions.Store = virtual.PGSessionStore{Pool: pool}
	syncer := &snapshot.Syncer{Pool: pool, Box: box, Log: log, MaxAuthorizationAge: maxStale, ListenerURL: config.Env("VMCP_DB_LISTENER_URL", ""),
		OnChange: func(snap *config.Snapshot) {
			sessions.Reconcile(snap)
			var configs []oauthtoken.Config
			for _, up := range snap.Upstreams {
				if up.OAuth != nil {
					configs = append(configs, *up.OAuth)
				}
			}
			oauthtoken.Retain(configs)
		},
	}
	if err := syncer.Start(ctx); err != nil {
		return err
	}

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
		policy := config.Env("VMCP_REDIS_FAILURE_POLICY", "local")
		if policy != "local" && policy != "strict" {
			return errors.New("VMCP_REDIS_FAILURE_POLICY must be local or strict")
		}
		limiter.(*ratelimit.FailOpen).Strict = policy == "strict"
		log.Info("rate limits are shared through Redis", "failure_policy", policy)
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

	gate := &lifecycle.Gate{}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		status := syncer.Status()
		readyCtx, cancel := context.WithTimeout(r.Context(), time.Second)
		defer cancel()
		var writable bool
		if err := pool.QueryRow(readyCtx, "SELECT NOT pg_is_in_recovery() AND current_setting('transaction_read_only') = 'off'").Scan(&writable); err != nil || !writable {
			status.Ready = false
			status.Reason = "audit_database_unavailable"
		}
		if redis, ok := limiter.(*ratelimit.FailOpen); ok && redis.Strict && redis.Check(readyCtx) != nil {
			status.Ready = false
			status.Reason = "shared_quota_unavailable"
		}
		if gate.Draining() {
			status.Ready = false
			status.Reason = "draining"
		}
		w.Header().Set("Content-Type", "application/json")
		if gate.Draining() || !status.Ready {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		_ = json.NewEncoder(w).Encode(struct {
			snapshot.Status
			Audit audit.Counters `json:"audit"`
		}{status, auditor.Stats()})
	})
	mux.HandleFunc("GET /audit-status", func(w http.ResponseWriter, r *http.Request) {
		pending, err := audit.Pending(r.Context(), pool)
		if err != nil {
			http.Error(w, "audit database unavailable", 503)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"process": auditor.Stats(), "durable": pending})
	})
	trusted, err := config.ParsePrefixes(config.Env("VMCP_TRUSTED_PROXIES", ""))
	if err != nil {
		return fmt.Errorf("invalid VMCP_TRUSTED_PROXIES: %w", err)
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
			Revocations: agentauth.PostgresRevocations{Pool: pool},
		})
		if err != nil {
			return fmt.Errorf("agent oauth: %w", err)
		}
		log.Info("accepting access tokens from an identity provider", "issuer", issuer)
	}
	deliveryStore := &delivery.Store{Pool: pool, Box: box, ListenerURL: syncer.ListenerURL}
	go deliveryStore.Run(ctx, func() int64 { return syncer.Status().Revision })
	approval, err := virtual.NewApprovalPolicy(config.Env("VMCP_APPROVAL_URL", ""))
	if err != nil {
		return err
	}
	metrics := telemetry.New()
	virtualHandler := &virtual.Handler{Metrics: metrics, Approval: approval, Snap: syncer, Sessions: sessions, Delivery: deliveryStore, Log: log, TrustedProxies: trusted, Audit: auditor, Breakers: breakers, Limiter: limiter,
		JWT: jwt, PublicURL: config.Env("VMCP_PUBLIC_URL", "")}
	virtualHandler.RunSubscriptions(ctx)
	virtualHandler.Register(mux)
	defer virtualHandler.StopWorkers()

	metrics.Gauges = func(ctx context.Context) map[string]float64 {
		status := syncer.Status()
		stats := pool.Stat()
		a := auditor.Stats()
		gauges := map[string]float64{"streams_active": float64(virtualHandler.ActiveStreams()), "sessions_active": float64(sessions.Len()), "snapshot_age_seconds": status.Age.Seconds(), "snapshot_revision": float64(status.Revision), "snapshot_refresh_seconds": syncer.RefreshDuration().Seconds(), "listener_reconnects_total": float64(syncer.ListenerReconnects()), "pool_acquired": float64(stats.AcquiredConns()), "pool_max": float64(stats.MaxConns()), "pool_acquire_seconds_total": stats.AcquireDuration().Seconds(), "pool_empty_acquires_total": float64(stats.EmptyAcquireCount()), "pool_cancelled_acquires_total": float64(stats.CanceledAcquireCount()), "audit_failed_writes_total": float64(a.FailedWrites), "audit_dropped_total": float64(a.Dropped), "audit_queue_pending": float64(a.Pending)}
		if pending, err := audit.Pending(ctx, pool); err == nil {
			gauges["audit_incomplete"] = float64(pending.Incomplete)
			gauges["audit_oldest_incomplete_seconds"] = pending.OldestIncompleteAgeSeconds
		}
		if redis, ok := limiter.(*ratelimit.FailOpen); ok {
			gauges["redis_failures_total"] = float64(redis.Failures.Load())
			if redis.Degraded.Load() {
				gauges["redis_degraded"] = 1
			} else {
				gauges["redis_degraded"] = 0
			}
		}
		states := breakers.CountStates()
		gauges["breakers_open"] = float64(states[resilience.Open])
		gauges["breakers_half_open"] = float64(states[resilience.HalfOpen])
		gauges["breakers_closed"] = float64(states[resilience.Closed])
		return gauges
	}
	shutdownTracing, err := telemetry.InitTracing(ctx, "vmcp-data-plane")
	if err != nil {
		return err
	}
	defer func() {
		traceCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = shutdownTracing(traceCtx)
	}()
	srv := &http.Server{
		Addr:              config.Env("VMCP_DP_ADDR", ":8080"),
		Handler:           gate.Handler(metrics.Handler(mux)),
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
	case <-signalCtx.Done():
	}

	gate.Stop()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), drainTimeout)
	defer cancel()
	shutdownErr := srv.Shutdown(shutdownCtx)
	if shutdownErr != nil {
		_ = srv.Close()
		finishCtx, finishCancel := context.WithTimeout(context.Background(), 6*time.Second)
		_ = gate.Wait(finishCtx)
		finishCancel()
	}
	if err := virtualHandler.WaitWorkers(shutdownCtx); err != nil {
		virtualHandler.StopWorkers()
		finishCtx, finishCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer finishCancel()
		_ = virtualHandler.WaitWorkers(finishCtx)
	}
	if shutdownErr != nil {
		return shutdownErr
	}

	log.Info("stopped")
	return nil
}
