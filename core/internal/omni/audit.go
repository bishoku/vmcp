package omni

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"
)

// AuditOptions are pinned by the data plane for one client call. No storage key
// or caller-supplied identity is accepted by this bridge.
type AuditOptions struct {
	Details           bool `json:"details"`
	Candidates        bool `json:"candidates"`
	Denied            bool `json:"denied"`
	EmbeddingRequest  bool `json:"embeddingRequest"`
	EmbeddingResponse bool `json:"embeddingResponse"`
	RerankRequest     bool `json:"rerankRequest"`
	RerankResponse    bool `json:"rerankResponse"`
	Limit             int  `json:"limit"`
}
type AuditOperation struct {
	Ticket  string       `json:"ticket"`
	ID      string       `json:"id"`
	Options AuditOptions `json:"options"`
}
type AuditStart struct {
	Tool    string          `json:"tool"`
	Payload json.RawMessage `json:"payload"`
}
type AuditFinish struct {
	Ticket        string          `json:"ticket"`
	Status        string          `json:"status"`
	Reason        string          `json:"reason"`
	Payload       []byte          `json:"payload"`
	OriginalBytes int             `json:"originalBytes"`
	DurationMS    int             `json:"durationMs"`
	Trace         *DiscoveryTrace `json:"trace,omitempty"`
}
type Auditor interface {
	Begin(context.Context, string, string, string, AuditStart) (AuditOperation, error)
	Complete(context.Context, string, string, string, AuditFinish) error
}
type GatewayAudit struct{ Backend *Backend }

func (a *GatewayAudit) Begin(ctx context.Context, token, ip, trace string, p AuditStart) (AuditOperation, error) {
	var out AuditOperation
	err := a.Backend.Request(ctx, "POST", "/internal/omni/audit/begin", token, ip, trace, p, &out)
	return out, err
}
func (a *GatewayAudit) Complete(ctx context.Context, token, ip, trace string, p AuditFinish) error {
	return a.Backend.Request(ctx, "POST", "/internal/omni/audit/complete", token, ip, trace, p, nil)
}

type auditedResponse struct {
	header   http.Header
	code     int
	body     bytes.Buffer
	overflow bool
}

func (w *auditedResponse) Header() http.Header { return w.header }
func (w *auditedResponse) WriteHeader(code int) {
	if w.code == 0 {
		w.code = code
	}
}
func (w *auditedResponse) Write(p []byte) (int, error) {
	if w.code == 0 {
		w.code = 200
	}
	if w.body.Len()+len(p) > 64<<20 {
		w.overflow = true
		return 0, io.ErrShortBuffer
	}
	return w.body.Write(p)
}

// Buffer the JSON response until strict completion is acknowledged. Never replay
// invoke_tool on an audit failure; its external action may already have executed.
func (s *Server) beginAudit(ctx context.Context, token, ip, trace string, msg *MessageAudit) (context.Context, func(http.ResponseWriter, *auditedResponse), error) {
	if s.Auditor == nil {
		return ctx, nil, nil
	}
	start := time.Now()
	op, err := s.Auditor.Begin(ctx, token, ip, trace, AuditStart{Tool: msg.Tool, Payload: msg.Payload})
	if err != nil {
		return ctx, nil, err
	}
	collector := newDiscoveryTrace(op.Options)
	ctx = context.WithValue(ctx, discoveryTraceKey{}, collector)
	ctx = context.WithValue(ctx, auditTicketKey{}, op.Ticket)
	done := func(w http.ResponseWriter, out *auditedResponse) {
		status, reason := "ok", ""
		if out.code >= 400 {
			status = "error"
			reason = "http_error"
			if out.code == 401 || out.code == 403 || out.code == 429 {
				status = "denied"
			}
			if out.code == 429 {
				reason = "rate_limited"
			}
		}
		var envelope struct {
			Error  json.RawMessage `json:"error"`
			Result struct {
				IsError bool `json:"isError"`
			} `json:"result"`
		}
		if json.Unmarshal(out.body.Bytes(), &envelope) == nil {
			if len(envelope.Error) > 0 {
				status = "error"
				reason = "rpc_error"
			}
			if envelope.Result.IsError {
				status = "tool_error"
			}
		}
		if status == "ok" && (msg.Tool == "discover_tools" || msg.Tool == "discover_server") && collector.Counts["returned_tools"]+collector.Counts["returned_servers"] == 0 {
			reason = "no_match"
		}
		payload := out.body.Bytes()
		original := len(payload)
		if original > 2<<20 {
			payload = nil
		}
		if out.overflow {
			status = "error"
			reason = "response_too_large"
		}
		completion, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		var diagnostics *DiscoveryTrace
		if len(collector.Stages) > 0 || len(collector.Counts) > 0 {
			diagnostics = collector
		}
		err := s.Auditor.Complete(completion, token, ip, trace, AuditFinish{Ticket: op.Ticket, Status: status, Reason: reason, Payload: payload, OriginalBytes: original, DurationMS: int(time.Since(start).Milliseconds()), Trace: diagnostics})
		if err != nil || out.overflow {
			http.Error(w, "audit outcome unavailable; operation may have completed; do not automatically retry", 503)
			return
		}
		for k, v := range out.header {
			w.Header()[k] = v
		}
		w.Header().Set("X-Request-Id", op.ID)
		if out.code == 0 {
			out.code = 200
		}
		w.WriteHeader(out.code)
		_, _ = w.Write(out.body.Bytes())
	}
	return ctx, done, nil
}

type MessageAudit struct {
	Tool    string
	Payload json.RawMessage
}
type auditTicketKey struct{}

func auditTicket(ctx context.Context) string { v, _ := ctx.Value(auditTicketKey{}).(string); return v }
