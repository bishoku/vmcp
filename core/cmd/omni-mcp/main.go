package main

import (
	"context"
	"errors"
	"log/slog"
	"math"
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
		dimensions, err := strconv.Atoi(config.Env("VMCP_OMNI_EMBEDDING_DIMENSIONS", "0"))
		if err != nil || dimensions < 0 || dimensions > 4096 {
			return errors.New("VMCP_OMNI_EMBEDDING_DIMENSIONS must be 0 (auto) or 1-4096")
		}
		textFormat := config.Env("VMCP_OMNI_EMBEDDING_TEXT_FORMAT", "legacy-v1")
		if textFormat != "legacy-v1" && textFormat != "tool-first-v1" {
			return errors.New("VMCP_OMNI_EMBEDDING_TEXT_FORMAT must be legacy-v1 or tool-first-v1")
		}
		e.Profile = omni.EmbeddingProfile{TextFormat: textFormat, Dimensions: dimensions, Revision: config.Env("VMCP_OMNI_EMBEDDING_REVISION", ""), QueryPrefix: config.Env("VMCP_OMNI_EMBEDDING_QUERY_PREFIX", ""), DocumentPrefix: config.Env("VMCP_OMNI_EMBEDDING_DOCUMENT_PREFIX", "")}
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
	slog.Info("omni embedding configuration", "enabled", embedder != nil, "tls_skip_verify", insecureEmbeddingTLS)
	minSimilarity, err := strconv.ParseFloat(config.Env("VMCP_OMNI_MIN_SIMILARITY", "0"), 64)
	if err != nil || math.IsNaN(minSimilarity) || minSimilarity < 0 || minSimilarity > 1 {
		return errors.New("VMCP_OMNI_MIN_SIMILARITY must be between 0 and 1")
	}
	index := omni.NewIndex(embedder)
	index.MinSimilarity = minSimilarity
	if endpoint := config.Env("VMCP_OMNI_RERANK_URL", ""); endpoint != "" {
		insecure, err := strconv.ParseBool(config.Env("VMCP_OMNI_RERANK_INSECURE_SKIP_VERIFY", "false"))
		if err != nil {
			return errors.New("VMCP_OMNI_RERANK_INSECURE_SKIP_VERIFY must be a boolean")
		}
		timeout, err := time.ParseDuration(config.Env("VMCP_OMNI_RERANK_TIMEOUT", "5s"))
		if err != nil {
			return errors.New("VMCP_OMNI_RERANK_TIMEOUT must be a duration")
		}
		reranker, err := omni.NewHTTPReranker(endpoint, config.Env("VMCP_OMNI_RERANK_MODEL", ""), config.Env("VMCP_OMNI_RERANK_KEY", ""), insecure, timeout)
		if err != nil {
			return err
		}
		candidates, err := strconv.Atoi(config.Env("VMCP_OMNI_RERANK_CANDIDATES", "20"))
		if err != nil || candidates < 1 || candidates > 100 {
			return errors.New("VMCP_OMNI_RERANK_CANDIDATES must be 1-100")
		}
		score, err := strconv.ParseFloat(config.Env("VMCP_OMNI_RERANK_MIN_SCORE", "0"), 64)
		if err != nil || math.IsNaN(score) || score < 0 || score > 1 {
			return errors.New("VMCP_OMNI_RERANK_MIN_SCORE must be between 0 and 1")
		}
		policy := config.Env("VMCP_OMNI_RERANK_FAILURE_POLICY", "fallback")
		if policy != "fallback" && policy != "closed" {
			return errors.New("VMCP_OMNI_RERANK_FAILURE_POLICY must be fallback or closed")
		}
		index.Reranker = reranker
		index.RerankCandidates = candidates
		index.RerankMinScore = score
		for name, target := range map[string]*float64{"VMCP_OMNI_RERANK_NO_MATCH_SCORE": &index.RerankNoMatchScore, "VMCP_OMNI_RERANK_CONFIDENCE_SCORE": &index.RerankConfidenceScore} {
			value, err := strconv.ParseFloat(config.Env(name, "0"), 64)
			if err != nil || math.IsNaN(value) || value < 0 || value > 1 {
				return errors.New(name + " must be between 0 and 1")
			}
			*target = value
		}
		index.RerankFailurePolicy = policy
		slog.Info("omni reranker configuration", "enabled", true, "candidates", candidates, "min_score", score, "no_match_score", index.RerankNoMatchScore, "confidence_score", index.RerankConfidenceScore, "timeout", timeout, "failure_policy", policy, "tls_skip_verify", insecure)
	} else {
		slog.Info("omni reranker configuration", "enabled", false)
	}
	go index.Run(ctx, backend, interval)
	limit, err := strconv.Atoi(config.Env("VMCP_OMNI_SEARCH_LIMIT", "120"))
	if err != nil || limit < 1 {
		return errors.New("VMCP_OMNI_SEARCH_LIMIT must be positive")
	}
	allowAnonymous, err := strconv.ParseBool(config.Env("VMCP_OMNI_ALLOW_ANONYMOUS", "false"))
	if err != nil {
		return errors.New("VMCP_OMNI_ALLOW_ANONYMOUS must be a boolean")
	}
	app := &omni.Server{Auditor: &omni.GatewayAudit{Backend: backend}, AllowAnonymous: allowAnonymous, SearchLimit: limit, Backend: backend, Index: index, TrustedProxies: proxies, PublicURL: config.Env("VMCP_OMNI_PUBLIC_URL", ""), Issuer: config.Env("VMCP_AGENT_OIDC_ISSUER", "")}
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
