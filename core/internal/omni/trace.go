package omni

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

type TraceStage struct {
	Name          string `json:"name"`
	Count         int    `json:"count,omitempty"`
	DurationMS    int64  `json:"durationMs,omitempty"`
	Status        string `json:"status,omitempty"`
	Details       any    `json:"details,omitempty"`
	Omitted       int    `json:"omitted,omitempty"`
	OriginalBytes int    `json:"originalBytes,omitempty"`
}
type DiscoveryTrace struct {
	Counts  map[string]int `json:"counts"`
	Stages  []TraceStage   `json:"stages"`
	Options AuditOptions   `json:"-"`
	bytes   int
}
type discoveryTraceKey struct{}

func newDiscoveryTrace(options AuditOptions) *DiscoveryTrace {
	if options.Limit < 1 || options.Limit > 200 {
		options.Limit = 100
	}
	return &DiscoveryTrace{Counts: map[string]int{}, Stages: []TraceStage{}, Options: options}
}
func traceFrom(ctx context.Context) *DiscoveryTrace {
	v, _ := ctx.Value(discoveryTraceKey{}).(*DiscoveryTrace)
	return v
}
func (t *DiscoveryTrace) add(stage TraceStage) {
	if t == nil {
		return
	}
	if len(t.Stages) >= 32 {
		return
	}
	raw, _ := json.Marshal(stage.Details)
	stage.OriginalBytes = max(stage.OriginalBytes, len(raw))
	provider := stage.Name == "embedding_request" || stage.Name == "embedding_response" || stage.Name == "reranker_request" || stage.Name == "reranker_response"
	if (!t.Options.Details && !provider) || len(raw) > 64<<10 || t.bytes+len(raw) > 512<<10 {
		if stage.Details != nil {
			stage.Omitted = max(stage.Omitted, stage.Count)
			stage.Details = nil
		}
	} else {
		t.bytes += len(raw)
	}
	t.Stages = append(t.Stages, stage)
}
func traceHits(ctx context.Context, name string, hits []Hit) {
	t := traceFrom(ctx)
	if t == nil {
		return
	}
	stage := TraceStage{Name: name, Count: len(hits)}
	if t.Options.Candidates && t.Options.Details {
		rows := []map[string]any{}
		for n, h := range hits[:min(len(hits), t.Options.Limit)] {
			rows = append(rows, map[string]any{"rank": n + 1, "tool_ref": h.Ref, "server_id": h.Server, "name": h.Name, "score": h.Score, "lexical_score": h.Lexical, "vector_score": h.VectorScore})
		}
		stage.Details = rows
		stage.Omitted = len(hits) - len(rows)
	}
	t.add(stage)
}

// Header values and endpoints never enter trace payloads. Replace the known model
// credential even if a provider unexpectedly echoes it in a JSON body.
func traceProvider(ctx context.Context, name, direction string, raw []byte, key string, status int) {
	t := traceFrom(ctx)
	if t == nil {
		return
	}
	enabled := name == "embedding" && ((direction == "request" && t.Options.EmbeddingRequest) || (direction == "response" && t.Options.EmbeddingResponse)) || name == "reranker" && ((direction == "request" && t.Options.RerankRequest) || (direction == "response" && t.Options.RerankResponse))
	if !enabled {
		return
	}
	stage := TraceStage{Name: name + "_" + direction, Status: "ok", Count: 1, OriginalBytes: len(raw)}
	if status >= 400 {
		stage.Status = "http_error"
	}
	if len(raw) <= 64<<10 {
		if key != "" {
			raw = []byte(strings.ReplaceAll(string(raw), key, "[REDACTED]"))
		}
		var value any
		if json.Unmarshal(raw, &value) == nil {
			stage.Details = value
		} else {
			stage.Status = "invalid_json"
		}
	} else {
		stage.Omitted = 1
	}
	t.add(stage)
}

// Tap the provider decoder's reads without changing its existing size limits.
type providerTraceBody struct {
	io.ReadCloser
	raw       []byte
	total     int
	ctx       context.Context
	name, key string
	status    int
}

func (b *providerTraceBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	b.total += n
	if len(b.raw)+n <= 64<<10 {
		b.raw = append(b.raw, p[:n]...)
	}
	return n, err
}
func (b *providerTraceBody) Close() error {
	if b.total > 64<<10 {
		if t := traceFrom(b.ctx); t != nil {
			t.add(TraceStage{Name: b.name + "_response", Count: 1, Omitted: 1, OriginalBytes: b.total, Status: "omitted_size"})
		}
	} else {
		traceProvider(b.ctx, b.name, "response", b.raw, b.key, b.status)
	}
	return b.ReadCloser.Close()
}
func tapProvider(ctx context.Context, name, key string, res *http.Response) {
	t := traceFrom(ctx)
	if t == nil {
		return
	}
	if (name == "embedding" && t.Options.EmbeddingResponse) || (name == "reranker" && t.Options.RerankResponse) {
		res.Body = &providerTraceBody{ReadCloser: res.Body, ctx: ctx, name: name, key: key, status: res.StatusCode}
	}
}

func traceCatalog(ctx context.Context, entries []entry, access Access, server, project string) {
	t := traceFrom(ctx)
	if t == nil {
		return
	}
	t.Counts["catalog_total"] = len(entries)
	denied := []map[string]string{}
	for _, e := range entries {
		reason := "eligible"
		if access.Refs[e.Ref] != e.Hash {
			reason = "unauthorized"
		} else if server != "" && e.Server != server {
			reason = "server_scope"
		} else if project != "" && e.Project != project {
			reason = "project_scope"
		}
		t.Counts[reason]++
		if reason == "unauthorized" && t.Options.Details && t.Options.Denied && len(denied) < t.Options.Limit {
			denied = append(denied, map[string]string{"tool_ref": e.Ref, "server_id": e.Server, "name": e.Name, "reason": reason})
		}
	}
	stage := TraceStage{Name: "authorization_and_scope", Count: t.Counts["eligible"]}
	if len(denied) > 0 {
		stage.Details = denied
		stage.Omitted = t.Counts["unauthorized"] - len(denied)
	}
	t.add(stage)
}
func traceCount(ctx context.Context, key string, n int) {
	if t := traceFrom(ctx); t != nil {
		t.Counts[key] += n
	}
}
