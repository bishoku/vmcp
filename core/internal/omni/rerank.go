package omni

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"sort"
	"time"
)

// Reranker returns one finite [0,1] relevance score per input document, in input
// order. It never supplies tool metadata or permission decisions.
type Reranker interface {
	Rerank(context.Context, string, []Document) ([]float64, error)
}
type HTTPReranker struct {
	Endpoint, Model, Key string
	Client               *http.Client
	TLSInsecure          bool
}

func NewHTTPReranker(endpoint, model, key string, insecure bool, timeout time.Duration) (*HTTPReranker, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") || model == "" {
		return nil, errors.New("valid rerank endpoint and model required")
	}
	if timeout <= 0 || timeout > 30*time.Second {
		return nil, errors.New("rerank timeout must be positive and at most 30s")
	}
	client, err := providerClient("VMCP_OMNI_RERANK_CA_FILE", insecure, timeout)
	if err != nil {
		return nil, err
	}
	return &HTTPReranker{Endpoint: endpoint, Model: model, Key: key, Client: client, TLSInsecure: insecure}, nil
}
func (e *HTTPReranker) failure(reason string, attrs ...slog.Attr) error {
	u, _ := url.Parse(e.Endpoint)
	attrs = append(attrs, slog.String("provider_origin", u.Scheme+"://"+u.Host))
	attrs = append(attrs, slog.Bool("tls_skip_verify", e.TLSInsecure))
	return &embeddingError{reason: reason, attrs: attrs}
}
func (e *HTTPReranker) Rerank(ctx context.Context, query string, documents []Document) ([]float64, error) {
	if len(documents) == 0 || len(documents) > 100 {
		return nil, e.failure("invalid_candidate_count", slog.Int("candidates", len(documents)))
	}
	texts := make([]string, len(documents))
	for n, d := range documents {
		texts[n] = d.StructuredText()
	}
	body, _ := json.Marshal(map[string]any{"model": e.Model, "query": query, "documents": texts, "top_n": len(texts)})
	req, err := http.NewRequestWithContext(ctx, "POST", e.Endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, e.failure("invalid_request")
	}
	req.Header.Set("Content-Type", "application/json")
	if e.Key != "" {
		req.Header.Set("Authorization", "Bearer "+e.Key)
	}
	traceProvider(ctx, "reranker", "request", body, e.Key, 0)
	res, err := e.Client.Do(req)
	if err != nil {
		return nil, e.failure(transportReason(err))
	}
	tapProvider(ctx, "reranker", e.Key, res)
	defer res.Body.Close()
	if res.StatusCode != 200 {
		attrs := []slog.Attr{slog.Int("http_status", res.StatusCode), slog.String("http_status_text", http.StatusText(res.StatusCode))}
		var payload struct {
			Error struct {
				Code any    `json:"code"`
				Type string `json:"type"`
			} `json:"error"`
		}
		if json.NewDecoder(io.LimitReader(res.Body, 16<<10)).Decode(&payload) == nil {
			sensitive := append(append([]string{}, texts...), query)
			for _, d := range documents {
				sensitive = append(sensitive, d.Name, d.Description, d.ServerName, d.ServerDescription)
			}
			if code, ok := payload.Error.Code.(string); ok && safeErrorToken(code, e.Key, sensitive) {
				attrs = append(attrs, slog.String("provider_code", code))
			}
			if safeErrorToken(payload.Error.Type, e.Key, sensitive) {
				attrs = append(attrs, slog.String("provider_type", payload.Error.Type))
			}
		}
		return nil, e.failure("http_status", attrs...)
	}
	var payload struct {
		Results []struct {
			Index *int     `json:"index"`
			Score *float64 `json:"relevance_score"`
		} `json:"results"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 2<<20)).Decode(&payload); err != nil {
		return nil, e.failure("invalid_rerank_json")
	}
	if len(payload.Results) != len(documents) {
		return nil, e.failure("rerank_count_mismatch", slog.Int("expected_count", len(documents)), slog.Int("actual_count", len(payload.Results)))
	}
	scores := make([]float64, len(documents))
	seen := make([]bool, len(documents))
	for _, row := range payload.Results {
		if row.Index == nil || *row.Index < 0 || *row.Index >= len(scores) || seen[*row.Index] || row.Score == nil {
			return nil, e.failure("invalid_rerank_result")
		}
		if math.IsNaN(*row.Score) || math.IsInf(*row.Score, 0) || *row.Score < 0 || *row.Score > 1 {
			return nil, e.failure("invalid_rerank_score", slog.Int("response_index", *row.Index), slog.String("expected_range", "0..1"))
		}
		seen[*row.Index] = true
		scores[*row.Index] = *row.Score
	}
	return scores, nil
}

type accessRefreshKey struct{}
type accessRefresh func(context.Context) (Access, error)

func (i *Index) refine(ctx context.Context, query string, hits []Hit, limit int, mode string) ([]Hit, string, error) {
	if i.Reranker == nil {
		return hits, mode, nil
	}
	// Refresh immediately before sending metadata to an external model. The server
	// rechecks results once more after reranking. The callback is request-scoped.
	if refresh, ok := ctx.Value(accessRefreshKey{}).(accessRefresh); ok {
		fresh, err := refresh(ctx)
		if err != nil {
			return nil, "", err
		}
		before := len(hits)
		hits = authorizedHits(hits, fresh)
		traceCount(ctx, "pre_rerank_acl_removed", before-len(hits))
	}
	if len(hits) == 0 {
		return []Hit{}, mode + "-reranked", nil
	}
	docs := make([]Document, len(hits))
	for n, h := range hits {
		docs[n] = h.Document
	}
	traceHits(ctx, "reranker_input", hits)
	started := time.Now()
	scores, err := i.Reranker.Rerank(ctx, query, docs)
	if err == nil {
		if len(scores) != len(hits) {
			err = &embeddingError{reason: "rerank_count_mismatch"}
		} else {
			for _, score := range scores {
				if math.IsNaN(score) || math.IsInf(score, 0) || score < 0 || score > 1 {
					err = &embeddingError{reason: "invalid_rerank_score"}
					break
				}
			}
		}
	}
	if trace := traceFrom(ctx); trace != nil {
		status := "ok"
		if err != nil {
			status = "provider_failure"
		}
		stage := TraceStage{Name: "reranker_scores", Count: len(hits), DurationMS: time.Since(started).Milliseconds(), Status: status}
		if trace.Options.Details && trace.Options.Candidates && err == nil {
			rows := []map[string]any{}
			for n, h := range hits[:min(len(hits), trace.Options.Limit)] {
				rows = append(rows, map[string]any{"tool_ref": h.Ref, "before_rank": n + 1, "retrieval_score": h.Score, "rerank_score": scores[n], "below_min_score": scores[n] < i.RerankMinScore})
			}
			stage.Details = map[string]any{"min_score": i.RerankMinScore, "no_match_score": i.RerankNoMatchScore, "confidence_score": i.RerankConfidenceScore, "candidates": rows}
			stage.Omitted = len(hits) - len(rows)
		}
		trace.add(stage)
	}
	if err != nil {
		if ctx.Err() != nil {
			return nil, "", ctx.Err()
		}
		i.rerankLog.report("search_rerank", err, "candidates", len(hits), "elapsed_ms", time.Since(started).Milliseconds(), "failure_policy", i.RerankFailurePolicy)
		if i.RerankFailurePolicy == "closed" {
			return nil, "", &HTTPError{503}
		}
		return finalizeHits(hits, limit), mode + "-rerank-fallback", nil
	}
	i.rerankLog.report("search_rerank", nil)
	bestScore := 0.0
	for _, score := range scores {
		bestScore = math.Max(bestScore, score)
	}
	if bestScore < i.RerankNoMatchScore {
		traceCount(ctx, "reranker_no_match", len(hits))
		return []Hit{}, mode + "-reranked-no-match", nil
	}
	uncertain := bestScore < i.RerankConfidenceScore
	if uncertain {
		traceCount(ctx, "uncertain_order_preserved", 1)
	}
	selected := make([]Hit, 0, len(hits))
	for n, h := range hits {
		if scores[n] >= i.RerankMinScore {
			if !uncertain {
				h.Score = scores[n]
			}
			selected = append(selected, h)
		}
	}
	traceCount(ctx, "reranker_min_score_removed", len(hits)-len(selected))
	traceHits(ctx, "reranker_selected", selected)
	if uncertain {
		return finalizeHits(selected, limit), mode + "-rerank-uncertain", nil
	}
	sort.Slice(selected, func(a, b int) bool { return better(selected[a], selected[b]) })
	return finalizeHits(selected, limit), mode + "-reranked", nil
}
func finalizeHits(hits []Hit, limit int) []Hit {
	out := make([]Hit, 0, min(limit, len(hits)))
	for _, h := range hits {
		out = append(out, h)
		if len(out) == limit {
			break
		}
	}
	return out
}
