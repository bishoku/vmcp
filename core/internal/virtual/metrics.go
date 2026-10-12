package virtual

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/bishoku/vmcp/core/internal/telemetry"
)

func operationOutcome(ctx context.Context, result json.RawMessage, err error) string {
	if err != nil {
		if ctx.Err() != nil {
			return "cancelled"
		}
		return "error"
	}
	var body struct {
		IsError bool `json:"isError"`
	}
	if json.Unmarshal(result, &body) == nil && body.IsError {
		return "tool_error"
	}
	return "ok"
}

type metricWriter struct {
	http.ResponseWriter
	status int
}

func (w *metricWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w *metricWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
		w.ResponseWriter.WriteHeader(status)
	}
}
func (w *metricWriter) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(200)
	}
	return w.ResponseWriter.Write(data)
}
func (w *metricWriter) Flush() {
	if w.status == 0 {
		w.WriteHeader(200)
	}
	_ = http.NewResponseController(w.ResponseWriter).Flush()
}

// HTTP requests for configured virtual servers, including auth failures. Tool
// semantics are recorded separately: MCP errors often use HTTP 200. SSE duration
// is the connection lifetime, not the duration of an individual operation.
func (h *Handler) observeVirtual(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.Metrics == nil {
			next(w, r)
			return
		}
		snap := h.Snap.Snapshot()
		if snap == nil {
			next(w, r)
			return
		}
		vs, ok := snap.VirtualServers[r.PathValue("slug")]
		if !ok {
			next(w, r)
			return
		}
		done := h.Metrics.BeginOperation(telemetry.Operation{Family: "virtual_requests", VirtualServer: vs.Slug})
		measured := &metricWriter{ResponseWriter: w}
		defer func() {
			outcome := "ok"
			if measured.status >= 400 {
				outcome = "error"
			}
			done(outcome)
		}()
		next(measured, r)
	}
}
