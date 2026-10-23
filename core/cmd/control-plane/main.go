package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/bishoku/vmcp/core/internal/audit"
	"github.com/bishoku/vmcp/core/internal/config"
	"github.com/bishoku/vmcp/core/internal/controlplane"
	"github.com/bishoku/vmcp/core/internal/lifecycle"
	"github.com/bishoku/vmcp/core/internal/netpolicy"
	"github.com/bishoku/vmcp/core/internal/oidc"
	"github.com/bishoku/vmcp/core/internal/secrets"
	"github.com/bishoku/vmcp/core/internal/store"
	"github.com/bishoku/vmcp/core/internal/telemetry"
	"github.com/jackc/pgx/v5/pgxpool"
)

const version = "1.0.2"

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("service", "control-plane")
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
	defer stop()
	ctx, cancelBg := context.WithCancel(context.Background())
	defer cancelBg()

	dbURL, err := config.MustEnv("VMCP_DATABASE_URL")
	if err != nil {
		return err
	}
	masterKey, err := config.MustEnv("VMCP_MASTER_KEY")
	if err != nil {
		return err
	}
	adminToken, err := config.MustEnv("VMCP_ADMIN_TOKEN")
	if err != nil {
		return err
	}
	box, err := secrets.FromEnv(masterKey)
	if err != nil {
		return err
	}

	if err := func() error {
		if config.Env("VMCP_SKIP_MIGRATIONS", "false") == "true" {
			return nil
		}
		return store.Migrate(ctx, dbURL)
	}(); err != nil {
		return err
	}
	pool, err := store.Connect(ctx, dbURL)
	if err != nil {
		return err
	}
	var bgWg sync.WaitGroup
	defer func() {
		cancelBg()
		bgWg.Wait()
		pool.Close()
	}()
	if err := store.CheckSchema(ctx, pool); err != nil {
		return err
	}

	// Audit retention: 0 keeps events forever.
	if days, err := strconv.Atoi(config.Env("VMCP_AUDIT_RETENTION_DAYS", "90")); err != nil || days < 0 {
		return errors.New("VMCP_AUDIT_RETENTION_DAYS must be a non-negative integer")
	} else {
		bgWg.Add(1)
		go func(d int) {
			defer bgWg.Done()
			purgeLoop(ctx, log, pool, time.Duration(d)*24*time.Hour)
		}(days)
	}

	provider, err := oidcFromEnv(ctx, masterKey, pool)
	if err != nil {
		return err
	}
	trusted, err := config.ParsePrefixes(config.Env("VMCP_TRUSTED_PROXIES", ""))
	if err != nil {
		return errors.New("invalid VMCP_TRUSTED_PROXIES: " + err.Error())
	}
	api := &controlplane.Server{
		OIDC:           provider,
		Store:          &store.Store{Pool: pool, Box: box, WireMockURL: strings.TrimSuffix(os.Getenv("VMCP_WIREMOCK_URL"), "/")},
		AdminToken:     adminToken,
		Version:        version,
		Log:            log,
		TrustedProxies: trusted,
	}
	defer func() { cancelBg(); bgWg.Wait(); api.CloseAudit() }()
	metrics := telemetry.New()
	api.Metrics = metrics
	bgWg.Add(1)
	go func() {
		defer bgWg.Done()
		api.RunDiscovery(ctx)
	}()
	if api.Store.WireMockURL != "" {
		bgWg.Add(1)
		go func() { defer bgWg.Done(); api.RunMockUpstreams(ctx) }()
	}
	metrics.Gauges = func(gctx context.Context) map[string]float64 {
		stats := pool.Stat()
		gauges := map[string]float64{
			"pool_acquired":                 float64(stats.AcquiredConns()),
			"pool_max":                      float64(stats.MaxConns()),
			"pool_acquire_seconds_total":    stats.AcquireDuration().Seconds(),
			"pool_empty_acquires_total":     float64(stats.EmptyAcquireCount()),
			"pool_cancelled_acquires_total": float64(stats.CanceledAcquireCount()),
		}
		if pending, err := audit.Pending(gctx, pool); err == nil {
			gauges["audit_incomplete"] = float64(pending.Incomplete)
			gauges["audit_oldest_incomplete_seconds"] = pending.OldestIncompleteAgeSeconds
		}
		return gauges
	}
	shutdownTracing, err := telemetry.InitTracing(ctx, "vmcp-control-plane")
	if err != nil {
		return err
	}
	defer func() {
		traceCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = shutdownTracing(traceCtx)
	}()
	gate := &lifecycle.Gate{}
	srv := &http.Server{
		Addr:              config.Env("VMCP_CP_ADDR", ":8081"),
		Handler:           gate.Handler(metrics.Handler(api.Handler())),
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      75 * time.Second,
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
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		_ = srv.Close()
		finishCtx, finishCancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer finishCancel()
		_ = gate.Wait(finishCtx)
		return err
	}
	log.Info("stopped")
	return nil
}

func purgeLoop(ctx context.Context, log *slog.Logger, pool *pgxpool.Pool, retention time.Duration) {
	t := time.NewTicker(15 * time.Minute)
	defer t.Stop()
	for {
		start := time.Now()
		var total int64
		for batch := 0; batch < 200 && ctx.Err() == nil; batch++ {
			payloadN, payloadErr := audit.PurgePayloads(ctx, pool)
			if payloadErr != nil && ctx.Err() == nil {
				log.Error("audit payload purge failed")
			}
			n, err := audit.Purge(ctx, pool, retention)
			if err != nil {
				if ctx.Err() == nil {
					log.Error("audit purge failed", "err", err)
				}
				break
			}
			total += n
			if n < 500 && payloadN < 500 {
				break
			}
		}
		if total > 0 {
			log.Info("audit events purged", "count", total, "duration_ms", time.Since(start).Milliseconds())
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// oidcFromEnv returns nil when VMCP_OIDC_ISSUER is unset (sign-in with tokens only).
func oidcFromEnv(ctx context.Context, masterKey string, pool *pgxpool.Pool) (*oidc.Provider, error) {
	issuer := config.Env("VMCP_OIDC_ISSUER", "")
	if issuer == "" {
		return nil, nil
	}
	clientID, err := config.MustEnv("VMCP_OIDC_CLIENT_ID")
	if err != nil {
		return nil, err
	}
	redirect, err := config.MustEnv("VMCP_OIDC_REDIRECT_URL")
	if err != nil {
		return nil, err
	}
	list := func(name string) []string {
		var out []string
		for _, g := range strings.Split(config.Env(name, ""), ",") {
			if g = strings.TrimSpace(g); g != "" {
				out = append(out, g)
			}
		}
		return out
	}
	// Cookies are signed with a key derived from the master key, so no extra secret to manage.
	key := sha256.Sum256([]byte("vmcp-session-v1:" + masterKey + ":" + store.TenantID()))
	return oidc.New(ctx, oidc.Config{
		Issuer:       issuer,
		DiscoveryURL: config.Env("VMCP_OIDC_DISCOVERY_URL", ""),
		ClientID:     clientID,
		ClientSecret: config.Env("VMCP_OIDC_CLIENT_SECRET", ""),
		RedirectURL:  redirect,
		GroupsClaim:  config.Env("VMCP_OIDC_GROUPS_CLAIM", "groups"),
		AdminGroups:  list("VMCP_OIDC_ADMIN_GROUPS"),
		EditorGroups: list("VMCP_OIDC_EDITOR_GROUPS"),
		ViewerGroups: list("VMCP_OIDC_VIEWER_GROUPS"),
		SessionKey:   key[:],
		Sessions:     oidc.PostgresSessions{Pool: pool},
	})
}
