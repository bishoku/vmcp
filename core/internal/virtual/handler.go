package virtual

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"math/rand/v2"
	"net/http"
	"net/netip"
	"strconv"
	"time"

	"github.com/bishoku/vmcp/core/internal/agentauth"
	"github.com/bishoku/vmcp/core/internal/audit"
	"github.com/bishoku/vmcp/core/internal/config"
	"github.com/bishoku/vmcp/core/internal/mcp"
	"github.com/bishoku/vmcp/core/internal/oauthtoken"
	"github.com/bishoku/vmcp/core/internal/ratelimit"
	"github.com/bishoku/vmcp/core/internal/resilience"
	"github.com/bishoku/vmcp/core/internal/upstream"
)

const (
	maxRequestBytes = 4 << 20
	callTimeout     = 60 * time.Second
)

// SnapshotProvider returns the current configuration snapshot.
type SnapshotProvider interface {
	Snapshot() *config.Snapshot
}

// Handler serves /mcp/v1/virtual/{slug}.
type Handler struct {
	Snap     SnapshotProvider
	Sessions *Manager
	Log      *slog.Logger

	// Audit receives one event per tool call and per denied request. Optional.
	Audit audit.Recorder

	// Limiter enforces calls per minute per agent key and per virtual server. Optional (nil = unlimited).
	Limiter ratelimit.Limiter

	// Breakers guards upstreams against repeated failures. Optional (nil disables).
	Breakers *resilience.Registry

	// TrustedProxies are the peers whose X-Forwarded-For header is believed.
	TrustedProxies []netip.Prefix

	// JWT verifies access tokens from an external identity provider. Optional (nil = agent keys only).
	JWT *agentauth.Verifier

	// PublicURL is the externally visible base URL, used in OAuth resource metadata. Optional.
	PublicURL string
}

// Register mounts the virtual server routes on mux.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /mcp/v1/virtual/{slug}", h.post)
	mux.HandleFunc("DELETE /mcp/v1/virtual/{slug}", h.delete)
	mux.HandleFunc("GET /.well-known/oauth-protected-resource/mcp/v1/virtual/{slug}", h.resourceMetadata)
	// No server-initiated stream yet: tell clients so, as the spec allows.
	mux.HandleFunc("GET /mcp/v1/virtual/{slug}", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Allow", "POST, DELETE")
		http.Error(w, "server-initiated stream not supported", http.StatusMethodNotAllowed)
	})
}

func (h *Handler) post(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	snap := h.Snap.Snapshot()
	vs, ok := snap.VirtualServers[slug]
	if !ok {
		http.NotFound(w, r)
		return
	}
	keySlug, subject, ok := h.authenticate(w, r, vs, snap)
	if !ok {
		return
	}
	ctx := withSubject(r.Context(), subject)

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxRequestBytes))
	if err != nil {
		http.Error(w, "request body too large or unreadable", http.StatusRequestEntityTooLarge)
		return
	}
	msg, err := mcp.Decode(body)
	if err != nil {
		var rpcErr *mcp.Error
		errors.As(err, &rpcErr)
		writeMessage(w, http.StatusBadRequest, &mcp.Message{JSONRPC: mcp.JSONRPCVersion, Error: rpcErr})
		return
	}

	if msg.IsRequest() && msg.Method == mcp.MethodInitialize {
		h.initialize(r.Context(), w, msg, vs, keySlug)
		return
	}

	sid := r.Header.Get("Mcp-Session-Id")
	if sid == "" {
		http.Error(w, "missing Mcp-Session-Id header", http.StatusBadRequest)
		return
	}
	sess, err := h.Sessions.Resolve(r.Context(), sid)
	if err != nil {
		h.Log.Error("session store unavailable", "err", err)
		http.Error(w, "session store unavailable", http.StatusServiceUnavailable)
		return
	}
	if sess == nil || sess.slug != slug || sess.keySlug != keySlug {
		http.Error(w, "unknown session", http.StatusNotFound)
		return
	}
	sess.touch()
	h.Sessions.Touch(r.Context(), sess)

	if !msg.IsRequest() {
		// Notifications and client responses are acknowledged and dropped for now.
		w.WriteHeader(http.StatusAccepted)
		return
	}

	switch msg.Method {
	case mcp.MethodPing:
		h.reply(w, msg.ID, struct{}{})
	case mcp.MethodToolsList:
		tools := make([]mcp.Tool, 0, len(vs.Tools))
		for _, t := range vs.Tools {
			tools = append(tools, t.Tool)
		}
		h.reply(w, msg.ID, mcp.ListToolsResult{Tools: tools})
	case mcp.MethodToolsCall:
		h.callTool(ctx, w, msg, vs, sess, snap, h.ipString(r))
	default:
		writeMessage(w, http.StatusOK, mcp.NewErrorResponse(msg.ID, mcp.CodeMethodNotFound, "method not found: "+msg.Method))
	}
}

func (h *Handler) initialize(ctx context.Context, w http.ResponseWriter, msg *mcp.Message, vs config.VirtualServer, keySlug string) {
	var p mcp.InitializeParams
	if err := json.Unmarshal(msg.Params, &p); err != nil {
		writeMessage(w, http.StatusOK, mcp.NewErrorResponse(msg.ID, mcp.CodeInvalidParams, "invalid initialize params"))
		return
	}
	version := mcp.NegotiateVersion(p.ProtocolVersion)
	sess, err := h.Sessions.Create(vs.Slug, version)
	if err != nil {
		writeMessage(w, http.StatusOK, mcp.NewErrorResponse(msg.ID, mcp.CodeInternalError, "cannot create session"))
		return
	}
	sess.keySlug = keySlug
	if err := h.Sessions.Persist(ctx, sess); err != nil {
		h.Sessions.Delete(sess.id)
		h.Log.Error("cannot persist session", "err", err)
		writeMessage(w, http.StatusOK, mcp.NewErrorResponse(msg.ID, mcp.CodeInternalError, "cannot create session"))
		return
	}
	w.Header().Set("Mcp-Session-Id", sess.id)
	h.reply(w, msg.ID, mcp.InitializeResult{
		ProtocolVersion: version,
		Capabilities:    mcp.ServerCapabilities{Tools: &mcp.ToolsCapability{ListChanged: false}},
		ServerInfo:      mcp.Implementation{Name: "vmcp:" + vs.Slug, Version: "0.1.0"},
	})
}

func (h *Handler) record(e audit.Event) {
	if h.Audit != nil {
		h.Audit.Record(e)
	}
}

func (h *Handler) ipString(r *http.Request) string {
	if ip, ok := h.clientIP(r); ok {
		return ip.String()
	}
	return ""
}

func (h *Handler) callTool(ctx context.Context, w http.ResponseWriter, msg *mcp.Message, vs config.VirtualServer, sess *Session, snap *config.Snapshot, ip string) {
	start := time.Now()
	ev := audit.Event{Time: start, VirtualServer: vs.Slug, AgentKey: sess.keySlug, ClientIP: ip, Subject: subjectFrom(ctx), Method: msg.Method}
	finish := func(status, reason string) {
		ev.Status, ev.Reason, ev.DurationMS = status, reason, int(time.Since(start).Milliseconds())
		h.record(ev)
	}
	var p mcp.CallToolParams
	if err := json.Unmarshal(msg.Params, &p); err != nil || p.Name == "" {
		finish("error", "invalid params")
		writeMessage(w, http.StatusOK, mcp.NewErrorResponse(msg.ID, mcp.CodeInvalidParams, "invalid tools/call params"))
		return
	}
	ev.Tool = p.Name
	if reason, blocked := vs.Blocked[p.Name]; blocked {
		finish("denied", reason)
		writeMessage(w, http.StatusOK, mcp.NewErrorResponse(msg.ID, mcp.CodeInvalidParams, "tool not permitted: "+p.Name))
		return
	}
	vt, ok := vs.Lookup(p.Name)
	if !ok {
		finish("error", "unknown tool")
		writeMessage(w, http.StatusOK, mcp.NewErrorResponse(msg.ID, mcp.CodeInvalidParams, "unknown tool: "+p.Name))
		return
	}
	if retry, limited := h.rateLimited(ctx, vs, snap, sess.keySlug); limited {
		secs := int(math.Ceil(retry.Seconds()))
		finish("denied", "rate limit")
		w.Header().Set("Retry-After", strconv.Itoa(max(secs, 1)))
		writeMessage(w, http.StatusTooManyRequests, mcp.NewErrorResponse(msg.ID, codeRateLimited, fmt.Sprintf("rate limit exceeded, retry in %ds", max(secs, 1))))
		return
	}
	up, ok := snap.Upstreams[vt.UpstreamID]
	if !ok {
		finish("error", "upstream unavailable")
		h.Log.Error("tool references missing upstream", "tool", p.Name, "upstream_id", vt.UpstreamID)
		writeMessage(w, http.StatusOK, mcp.NewErrorResponse(msg.ID, mcp.CodeInternalError, "upstream unavailable"))
		return
	}

	ev.Upstream = up.Slug
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()

	args := p.Arguments
	if vs.PII != nil && vs.PII.MaskArguments {
		masked, n, err := vs.PII.MaskJSON(args)
		if err != nil {
			finish("error", "invalid arguments")
			writeMessage(w, http.StatusOK, mcp.NewErrorResponse(msg.ID, mcp.CodeInvalidParams, "arguments are not valid JSON"))
			return
		}
		args, ev.Redactions = masked, n
	}
	result, err := h.callResilient(ctx, sess, up, vt, args)
	if err != nil {
		if errors.Is(err, errCircuitOpen) {
			finish("error", "circuit open")
			writeMessage(w, http.StatusOK, mcp.NewErrorResponse(msg.ID, mcp.CodeInternalError, "upstream temporarily unavailable"))
			return
		}
		var rpcErr *mcp.Error
		if errors.As(err, &rpcErr) {
			// The upstream answered with a JSON-RPC error: relay it, minus any PII in its message.
			if vs.PII != nil {
				var n int
				masked := *rpcErr
				masked.Message, n = vs.PII.MaskString(rpcErr.Message)
				rpcErr, ev.Redactions = &masked, ev.Redactions+n
			}
			finish("error", "upstream rpc error")
			writeMessage(w, http.StatusOK, &mcp.Message{JSONRPC: mcp.JSONRPCVersion, ID: msg.ID, Error: rpcErr})
			return
		}
		finish("error", "upstream unavailable")
		h.Log.Error("upstream call failed", "upstream", up.Slug, "tool", vt.UpstreamName, "err", err)
		writeMessage(w, http.StatusOK, mcp.NewErrorResponse(msg.ID, mcp.CodeInternalError, "upstream unavailable"))
		return
	}
	if vs.PII != nil {
		masked, n, err := vs.PII.MaskJSON(result)
		if err != nil {
			// Never forward a result we could not inspect.
			finish("error", "result could not be masked")
			writeMessage(w, http.StatusOK, mcp.NewErrorResponse(msg.ID, mcp.CodeInternalError, "upstream result could not be processed"))
			return
		}
		result, ev.Redactions = masked, ev.Redactions+n
	}
	var res struct {
		IsError bool `json:"isError"`
	}
	_ = json.Unmarshal(result, &res)
	if res.IsError {
		finish("tool_error", "")
	} else {
		finish("ok", "")
	}
	writeMessage(w, http.StatusOK, &mcp.Message{JSONRPC: mcp.JSONRPCVersion, ID: msg.ID, Result: result})
}

// codeRateLimited is a JSON-RPC server error (the -32000..-32099 range is implementation defined).
const codeRateLimited = -32029

// rateLimited charges one call to the key's and the virtual server's budget. A limiter that
// cannot answer never blocks a call.
func (h *Handler) rateLimited(ctx context.Context, vs config.VirtualServer, snap *config.Snapshot, keySlug string) (time.Duration, bool) {
	if h.Limiter == nil {
		return 0, false
	}
	if lim := snap.KeyLimits[keySlug]; keySlug != "" && lim > 0 {
		if ok, retry, err := h.Limiter.Allow(ctx, "key:"+keySlug, lim); err == nil && !ok {
			return retry, true
		}
	}
	if vs.RateLimit > 0 {
		if ok, retry, err := h.Limiter.Allow(ctx, "vs:"+vs.Slug, vs.RateLimit); err == nil && !ok {
			return retry, true
		}
	}
	return 0, false
}

var errCircuitOpen = errors.New("circuit open")

// idempotent reports whether the upstream declares the tool safe to repeat.
func idempotent(annotations json.RawMessage) bool {
	var a struct {
		ReadOnly   bool `json:"readOnlyHint"`
		Idempotent bool `json:"idempotentHint"`
	}
	return json.Unmarshal(annotations, &a) == nil && (a.ReadOnly || a.Idempotent)
}

// callResilient adds the circuit breaker and, for idempotent tools only, one retry with jitter
// after a transport failure. Replies from the upstream (including JSON-RPC errors) prove it is
// alive and never count as failures.
func (h *Handler) callResilient(ctx context.Context, sess *Session, up config.Upstream, vt config.VirtualTool, args json.RawMessage) (json.RawMessage, error) {
	br := h.Breakers.Get(up.ID)
	attempts := 1
	if idempotent(vt.Tool.Annotations) {
		attempts = 2
	}
	var res json.RawMessage
	var err error
	for i := 0; i < attempts; i++ {
		if i > 0 {
			select {
			case <-ctx.Done():
				return nil, err
			case <-time.After(50*time.Millisecond + rand.N(200*time.Millisecond)):
			}
		}
		if br != nil && !br.Allow() {
			return nil, errCircuitOpen
		}
		res, err = h.callUpstream(ctx, sess, up, vt.UpstreamName, args)
		var rpcErr *mcp.Error
		switch {
		case err == nil || errors.As(err, &rpcErr):
			if br != nil {
				br.Success()
			}
			return res, err
		case ctx.Err() != nil:
			return res, err // the caller gave up or our deadline hit: not evidence about the upstream
		case oauthtoken.IsTokenError(err):
			return res, err // the identity provider failed, not the upstream: no breaker hit, no retry
		}
		if br != nil {
			br.Failure()
		}
	}
	return res, err
}

// callUpstream calls the tool, re-establishing the upstream session once if it expired.
func (h *Handler) callUpstream(ctx context.Context, sess *Session, up config.Upstream, tool string, args json.RawMessage) (json.RawMessage, error) {
	for attempt := 0; ; attempt++ {
		c, err := sess.client(ctx, up)
		if err != nil {
			return nil, err
		}
		res, err := c.CallTool(ctx, tool, args)
		if errors.Is(err, upstream.ErrSessionExpired) && attempt == 0 {
			sess.reset(up.ID, c)
			continue
		}
		return res, err
	}
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	snap := h.Snap.Snapshot()
	vs, ok := snap.VirtualServers[r.PathValue("slug")]
	if !ok {
		http.NotFound(w, r)
		return
	}
	keySlug, _, ok := h.authenticate(w, r, vs, snap)
	if !ok {
		return
	}
	sess, err := h.Sessions.Resolve(r.Context(), r.Header.Get("Mcp-Session-Id"))
	if err != nil {
		http.Error(w, "session store unavailable", http.StatusServiceUnavailable)
		return
	}
	if sess == nil || sess.slug != r.PathValue("slug") || sess.keySlug != keySlug {
		http.Error(w, "unknown session", http.StatusNotFound)
		return
	}
	h.Sessions.End(r.Context(), sess.id)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) reply(w http.ResponseWriter, id mcp.ID, result any) {
	m, err := mcp.NewResult(id, result)
	if err != nil {
		m = mcp.NewErrorResponse(id, mcp.CodeInternalError, "encode result")
	}
	writeMessage(w, http.StatusOK, m)
}

func writeMessage(w http.ResponseWriter, status int, m *mcp.Message) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(m)
}
