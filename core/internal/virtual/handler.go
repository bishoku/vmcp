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
	"sync"
	"sync/atomic"
	"time"

	"github.com/bishoku/vmcp/core/internal/agentauth"
	"github.com/bishoku/vmcp/core/internal/audit"
	"github.com/bishoku/vmcp/core/internal/callercontext"
	"github.com/bishoku/vmcp/core/internal/config"
	"github.com/bishoku/vmcp/core/internal/delivery"
	"github.com/bishoku/vmcp/core/internal/mcp"
	"github.com/bishoku/vmcp/core/internal/netpolicy"
	"github.com/bishoku/vmcp/core/internal/oauthtoken"
	"github.com/bishoku/vmcp/core/internal/ratelimit"
	"github.com/bishoku/vmcp/core/internal/resilience"
	"github.com/bishoku/vmcp/core/internal/restapi"
	"github.com/bishoku/vmcp/core/internal/telemetry"
	"github.com/bishoku/vmcp/core/internal/upstream"
)

const (
	maxRequestBytes = 4 << 20
	callTimeout     = 60 * time.Second
)

// SnapshotProvider supplies inspection and freshness-checked authorization views.
type SnapshotProvider interface {
	Snapshot() *config.Snapshot
	AuthorizedSnapshot() (*config.Snapshot, error)
}

// Handler serves /mcp/v1/virtual/{slug}.
type Handler struct {
	OmniAllowAnonymous bool
	omniMu             sync.Mutex
	omniPublished      *omniPublishedCatalog
	Metrics            *telemetry.Registry
	Approval           *ApprovalPolicy
	Snap               SnapshotProvider
	Sessions           *Manager
	Delivery           *delivery.Store
	Log                *slog.Logger

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
	PublicURL     string
	activeStreams atomic.Int64
	workerWG      sync.WaitGroup
	workerMu      sync.Mutex
	workerStopped bool
	workerOnce    sync.Once
	workerCtx     context.Context
	workerCancel  context.CancelFunc
	budgetMu      sync.Mutex
	budgets       map[string]int
}

// Register mounts the virtual server routes on mux.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /mcp/v1/virtual/{slug}", ingress(h.observeVirtual(h.post)))
	mux.HandleFunc("DELETE /mcp/v1/virtual/{slug}", ingress(h.observeVirtual(h.delete)))
	mux.HandleFunc("GET /.well-known/oauth-protected-resource/mcp/v1/virtual/{slug}", h.resourceMetadata)
	// No server-initiated stream yet: tell clients so, as the spec allows.
	mux.HandleFunc("GET /mcp/v1/virtual/{slug}", ingress(h.observeVirtual(func(w http.ResponseWriter, r *http.Request) {
		if h.Delivery != nil {
			h.getStream(w, r)
			return
		}
		snap, fresh := h.requestSnapshot(w, r)
		if !fresh {
			return
		}
		vs, ok := snap.VirtualServers[r.PathValue("slug")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		owner, ok := h.authenticate(w, r, vs, snap)
		if !ok {
			return
		}
		sid := r.Header.Get("Mcp-Session-Id")
		if sid != "" {
			sess, err := h.Sessions.Resolve(r.Context(), sid)
			if err != nil {
				http.Error(w, "session store unavailable", 503)
				return
			}
			if sess == nil || sess.slug != vs.Slug || sess.owner != owner {
				http.Error(w, "unknown session", 404)
				return
			}
			if !validSessionVersion(w, r, sess) {
				return
			}
		}
		w.Header().Set("Allow", "POST, DELETE")
		http.Error(w, "server-initiated stream not supported", http.StatusMethodNotAllowed)
	})))
}

func (h *Handler) post(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	snap, fresh := h.requestSnapshot(w, r)
	if !fresh {
		return
	}
	initialRev := snap.Revision
	vs, ok := snap.VirtualServers[slug]
	if !ok {
		http.NotFound(w, r)
		return
	}
	owner, ok := h.authenticate(w, r, vs, snap)
	if !ok {
		return
	}
	authAt := time.Now()
	ctx := audit.WithTrace(withSubject(r.Context(), owner.Subject), r.Header.Get("traceparent"))

	release, admitted := h.acquire(owner, slug, h.ipString(r))
	if !admitted {
		http.Error(w, "request concurrency limit exceeded", 429)
		return
	}
	defer release()
	controller := http.NewResponseController(w)
	_ = controller.SetReadDeadline(time.Now().Add(10 * time.Second))
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxRequestBytes))
	if err != nil {
		http.Error(w, "request body too large or unreadable", http.StatusRequestEntityTooLarge)
		return
	}
	_ = controller.SetReadDeadline(time.Time{})
	msg, err := mcp.Decode(body)
	if err != nil {
		var rpcErr *mcp.Error
		errors.As(err, &rpcErr)
		writeMessage(w, http.StatusBadRequest, &mcp.Message{JSONRPC: mcp.JSONRPCVersion, Error: rpcErr})
		return
	}

	// Reading a body may outlive the freshness window or a token's lifetime.
	snap, fresh = h.requestSnapshot(w, r)
	if !fresh {
		return
	}
	vs, ok = snap.VirtualServers[slug]
	if !ok {
		http.NotFound(w, r)
		return
	}
	if snap.Revision != initialRev || time.Since(authAt) >= 25*time.Millisecond {
		currentOwner, ok := h.authenticate(w, r, vs, snap)
		if !ok {
			return
		}
		if currentOwner != owner {
			http.Error(w, "authorization changed; retry", http.StatusConflict)
			return
		}
	}
	var lastVerified atomic.Int64
	ctx = context.WithValue(ctx, dialHookKey{}, func() {
		lastVerified.Store(0)
	})
	ctx = context.WithValue(ctx, authorizationGuardKey{}, func(dispatchCtx context.Context) error {
		request := r.WithContext(dispatchCtx)
		current, ok := h.requestSnapshot(w, request)
		if !ok {
			return errAuthorizationChanged
		}
		if current.Revision != snap.Revision {
			http.Error(w, "configuration changed; retry", http.StatusConflict)
			return errAuthorizationChanged
		}
		sid := r.Header.Get("Mcp-Session-Id")
		if sid != "" && h.Sessions.Store == nil {
			if live := h.Sessions.Get(sid); live == nil || live.owner != owner {
				http.Error(w, "unknown session", 404)
				return errAuthorizationChanged
			}
		}
		if last := lastVerified.Load(); last > 0 && time.Since(time.Unix(0, last)) < 50*time.Millisecond {
			return nil
		}
		verified, ok := h.authenticate(w, request, vs, current)
		if !ok {
			return errAuthorizationChanged
		}
		if verified != owner {
			http.Error(w, "authorization changed; retry", http.StatusConflict)
			return errAuthorizationChanged
		}
		if sid != "" {
			live, err := h.Sessions.Resolve(dispatchCtx, sid)
			if err != nil {
				http.Error(w, "session store unavailable", 503)
				return errAuthorizationChanged
			}
			if live == nil || live.owner != owner {
				http.Error(w, "unknown session", 404)
				return errAuthorizationChanged
			}
		}
		lastVerified.Store(time.Now().UnixNano())
		return nil
	})

	if msg.IsRequest() && msg.Method == mcp.MethodInitialize {
		if r.Header.Get("Mcp-Session-Id") != "" {
			writeMessage(w, http.StatusOK, mcp.NewErrorResponse(msg.ID, mcp.CodeInvalidRequest, "initialize must start a new session"))
			return
		}
		h.initialize(r.Context(), w, msg, vs, owner)
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
	if sess == nil || sess.slug != slug || sess.owner != owner {
		http.Error(w, "unknown session", http.StatusNotFound)
		return
	}
	lastVerified.Store(time.Now().UnixNano())
	if err := checkAuthorization(ctx); err != nil {
		return
	}
	sess.touch()
	h.Sessions.Touch(r.Context(), sess)

	if !validSessionVersion(w, r, sess) {
		return
	}
	if msg.IsNotification() && msg.Method == mcp.MethodInitialized {
		if err := h.Sessions.Initialize(r.Context(), sess); err != nil {
			http.Error(w, "session store unavailable", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusAccepted)
		return
	}
	ready, err := h.Sessions.Ready(r.Context(), sess)
	if err != nil {
		http.Error(w, "session store unavailable", http.StatusServiceUnavailable)
		return
	}
	if !ready && msg.Method != mcp.MethodPing {
		if msg.IsRequest() {
			writeMessage(w, http.StatusOK, mcp.NewErrorResponse(msg.ID, mcp.CodeInvalidRequest, "send notifications/initialized before normal operations"))
		} else {
			http.Error(w, "session not initialized", http.StatusBadRequest)
		}
		return
	}
	if h.Delivery != nil && h.deliveryInput(w, r, sess, msg, vs) {
		return
	}
	if !msg.IsRequest() {
		if msg.IsResponse() {
			http.Error(w, "no pending server request", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusAccepted)
		return
	}

	switch msg.Method {
	case "logging/setLevel":
		if h.Delivery == nil {
			writeMessage(w, 200, mcp.NewErrorResponse(msg.ID, mcp.CodeMethodNotFound, "logging unavailable"))
			return
		}
		var p struct {
			Level string `json:"level"`
		}
		if json.Unmarshal(msg.Params, &p) != nil {
			writeMessage(w, 200, mcp.NewErrorResponse(msg.ID, mcp.CodeInvalidParams, "invalid logging level"))
			return
		}
		if _, ok := logLevels[p.Level]; !ok {
			writeMessage(w, 200, mcp.NewErrorResponse(msg.ID, mcp.CodeInvalidParams, "invalid logging level"))
			return
		}
		if h.auditDelivery(ctx, sess, "", "mcp.logging_set_level") != nil {
			http.Error(w, "audit unavailable", 503)
			return
		}
		if _, err := h.Delivery.Pool.Exec(ctx, `UPDATE sessions SET logging_level=$2 WHERE id=$1 AND terminated_at IS NULL`, sess.id, p.Level); err != nil {
			http.Error(w, "session unavailable", 503)
			return
		}
		sess.SetLoggingLevel(p.Level)
		h.reply(w, msg.ID, struct{}{})
	case mcp.MethodPing:
		h.reply(w, msg.ID, struct{}{})
	case mcp.MethodToolsList:
		h.listTools(w, msg, vs, snap.Revision)
	case "resources/list":
		h.listCatalog(w, msg, vs, snap.Revision, "resource", "resources")
	case "resources/templates/list":
		h.listCatalog(w, msg, vs, snap.Revision, "template", "resourceTemplates")
	case "prompts/list":
		h.listCatalog(w, msg, vs, snap.Revision, "prompt", "prompts")
	case "resources/read", "resources/subscribe", "resources/unsubscribe", "prompts/get", "completion/complete":
		if h.Delivery != nil {
			h.streamCall(ctx, w, r, msg, vs, sess, snap)
			return
		}
		h.catalogCall(ctx, w, msg, vs, sess, snap, h.ipString(r))
	case mcp.MethodToolsCall:
		if h.Delivery != nil {
			h.streamCall(ctx, w, r, msg, vs, sess, snap)
			return
		}
		h.callTool(ctx, w, msg, vs, sess, snap, h.ipString(r), r.Header)
	default:
		writeMessage(w, http.StatusOK, mcp.NewErrorResponse(msg.ID, mcp.CodeMethodNotFound, "method not found: "+msg.Method))
	}
}

func (h *Handler) initialize(ctx context.Context, w http.ResponseWriter, msg *mcp.Message, vs config.VirtualServer, owner Owner) {
	p, err := mcp.ValidateInitializeParams(msg.Params)
	if err != nil {
		writeMessage(w, http.StatusOK, mcp.NewErrorResponse(msg.ID, mcp.CodeInvalidParams, "invalid initialize params: "+err.Error()))
		return
	}

	version := mcp.NegotiateVersion(p.ProtocolVersion)
	sess, err := h.Sessions.Create(vs.Slug, version)
	if err != nil {
		writeMessage(w, http.StatusOK, mcp.NewErrorResponse(msg.ID, mcp.CodeInternalError, "cannot create session"))
		return
	}
	sess.clientCapabilities = append(json.RawMessage(nil), p.Capabilities...)
	sess.keySlug = owner.Principal
	sess.owner = owner
	if err := h.Sessions.Persist(ctx, sess); err != nil {
		h.Sessions.Delete(sess.id)
		h.Log.Error("cannot persist session", "err", err)
		writeMessage(w, http.StatusOK, mcp.NewErrorResponse(msg.ID, mcp.CodeInternalError, "cannot create session"))
		return
	}
	w.Header().Set("Mcp-Session-Id", sess.id)
	caps := vs.CatalogCapabilities()
	if h.Delivery != nil {
		caps.Logging = &struct{}{}
	}
	caps.Tools = &mcp.ToolsCapability{ListChanged: h.Delivery != nil}
	if caps.Resources != nil {
		caps.Resources.ListChanged = h.Delivery != nil
		caps.Resources.Subscribe = caps.Resources.Subscribe && h.Delivery != nil
	}
	if caps.Prompts != nil {
		caps.Prompts.ListChanged = h.Delivery != nil
	}
	h.reply(w, msg.ID, mcp.InitializeResult{
		Instructions:    vs.Instructions,
		ProtocolVersion: version,
		Capabilities:    caps,
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

func (h *Handler) callTool(ctx context.Context, w http.ResponseWriter, msg *mcp.Message, vs config.VirtualServer, sess *Session, snap *config.Snapshot, ip string, incoming http.Header) {
	start := time.Now()
	ev := audit.Event{Time: start, VirtualServer: vs.Slug, AgentKey: sess.keySlug, ClientIP: ip, Subject: subjectFrom(ctx), Method: msg.Method,
		ActorType: sess.owner.Mode, ActorID: sess.owner.CredentialID, Issuer: sess.owner.Issuer, Revision: snap.Revision, Action: "tools/call", Target: vs.Slug, TraceID: audit.TraceFrom(ctx)}
	if via, _ := ctx.Value(omniContextKey{}).(bool); via {
		ev.Action = "omni/tools/call"
	}
	ev.Scope = "virtual"
	ev.Boundary = "virtual"
	ev.RawPayload = msg.Params
	if _, ok := h.Audit.(*audit.Runtime); ok {
		policy := snap.Audit
		ev.Policy = &policy
	}
	if requestID, ok := ctx.Value(deliveryRequestKey{}).(string); ok {
		ev.RequestID = requestID
	}
	var responsePayload json.RawMessage
	finish := func(status, reason string) bool {
		ev.Status, ev.Reason, ev.DurationMS = status, reason, int(time.Since(start).Milliseconds())
		if status == "error" || status == "denied" {
			ev.RawPayload, _ = json.Marshal(map[string]string{"error": reason})
		}
		if len(responsePayload) > 0 {
			ev.RawPayload = responsePayload
		}
		if durable, ok := h.Audit.(audit.DurableRecorder); ok && ev.OperationID != "" {
			completion, cancel := context.WithTimeout(context.WithoutCancel(ctx), audit.WriteTimeout)
			defer cancel()
			if err := durable.Complete(completion, ev); err != nil {
				ev.Status = "error"
				h.Log.Error("audit outcome unavailable; operation incomplete", "operation_id", ev.OperationID)
				return false
			}
		} else {
			h.record(ev)
		}
		return true
	}
	var p mcp.CallToolParams
	if err := json.Unmarshal(msg.Params, &p); err != nil || p.Name == "" {
		if !finish("error", "invalid params") {
			http.Error(w, "audit outcome unavailable; operation may have completed", 503)
			return
		}
		writeMessage(w, http.StatusOK, mcp.NewErrorResponse(msg.ID, mcp.CodeInvalidParams, "invalid tools/call params"))
		return
	}
	if reason, blocked := vs.Blocked[p.Name]; blocked {
		done := h.Metrics.BeginOperation(telemetry.Operation{Family: "virtual_tool_calls", VirtualServer: vs.Slug, Tool: p.Name})
		defer func() { done(ev.Status) }()
		if !finish("denied", reason) {
			http.Error(w, "audit outcome unavailable; operation may have completed", 503)
			return
		}
		writeMessage(w, http.StatusOK, mcp.NewErrorResponse(msg.ID, mcp.CodeInvalidParams, "tool not permitted: "+p.Name))
		return
	}
	vt, ok := vs.Lookup(p.Name)
	if !ok {
		if !finish("error", "unknown tool") {
			http.Error(w, "audit outcome unavailable; operation may have completed", 503)
			return
		}
		writeMessage(w, http.StatusOK, mcp.NewErrorResponse(msg.ID, mcp.CodeInvalidParams, "unknown tool: "+p.Name))
		return
	}
	ev.Tool = vt.Tool.Name
	done := h.Metrics.BeginOperation(telemetry.Operation{Family: "virtual_tool_calls", VirtualServer: vs.Slug, Tool: vt.Tool.Name})
	defer func() { done(ev.Status) }()
	if retry, limited := h.rateLimited(ctx, vs, snap, sess.keySlug); limited {
		secs := int(math.Ceil(retry.Seconds()))
		if !finish("denied", "rate limit") {
			http.Error(w, "audit outcome unavailable; operation may have completed", 503)
			return
		}
		w.Header().Set("Retry-After", strconv.Itoa(max(secs, 1)))
		writeMessage(w, http.StatusTooManyRequests, mcp.NewErrorResponse(msg.ID, codeRateLimited, fmt.Sprintf("rate limit exceeded, retry in %ds", max(secs, 1))))
		return
	}
	up, ok := snap.Upstreams[vt.UpstreamID]
	if !ok {
		if !finish("error", "upstream unavailable") {
			http.Error(w, "audit outcome unavailable; operation may have completed", 503)
			return
		}
		h.Log.Error("tool references missing upstream", "tool", p.Name, "upstream_id", vt.UpstreamID)
		writeMessage(w, http.StatusOK, mcp.NewErrorResponse(msg.ID, mcp.CodeInternalError, "upstream unavailable"))
		return
	}

	selectedMeta, dynamicHeaders, contextErr := up.CallerContext.Prepare(p.Meta, up.Kind)
	if contextErr != nil {
		if !finish("error", contextErr.Error()) {
			http.Error(w, "audit outcome unavailable", 503)
			return
		}
		writeMessage(w, http.StatusOK, mcp.NewErrorResponse(msg.ID, mcp.CodeInvalidParams, contextErr.Error()))
		return
	}
	forwardedHeaders, headerErr := callercontext.PrepareHeaders(up.RequestHeaderMappings, incoming)
	if headerErr != nil {
		if !finish("error", headerErr.Error()) {
			http.Error(w, "audit outcome unavailable", 503)
			return
		}
		writeMessage(w, http.StatusOK, mcp.NewErrorResponse(msg.ID, mcp.CodeInvalidParams, headerErr.Error()))
		return
	}
	if dynamicHeaders == nil {
		dynamicHeaders = http.Header{}
	}
	for name, values := range forwardedHeaders {
		dynamicHeaders[name] = values
	}
	ctx = upstream.WithCallerMeta(ctx, selectedMeta)
	ctx = callercontext.WithHeaders(ctx, dynamicHeaders)
	ev.Upstream = up.Slug
	if err := h.Approval.Check(ctx, PolicyDecision{Actor: sess.owner.CredentialID, Server: vs.Slug, Method: msg.Method, Object: p.Name, Revision: snap.Revision}, p.Arguments); err != nil {
		if !finish("denied", "approval policy") {
			http.Error(w, "audit outcome unavailable", 503)
			return
		}
		writeMessage(w, http.StatusOK, mcp.NewErrorResponse(msg.ID, -32003, "approval policy denied operation"))
		return
	}
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()

	args := p.Arguments
	if vs.PII != nil && vs.PII.MaskArguments {
		masked, n, err := vs.PII.MaskJSON(args)
		if err != nil {
			if !finish("error", "invalid arguments") {
				http.Error(w, "audit outcome unavailable; operation may have completed", 503)
				return
			}
			writeMessage(w, http.StatusOK, mcp.NewErrorResponse(msg.ID, mcp.CodeInvalidParams, "arguments are not valid JSON"))
			return
		}
		args, ev.Redactions = masked, n
	}
	if durable, ok := h.Audit.(audit.DurableRecorder); ok {
		var err error
		ev, err = durable.Admit(ctx, ev)
		if ev.RequestID != "" {
			w.Header().Set("X-Request-Id", ev.RequestID)
		}
		if err != nil {
			http.Error(w, "audit admission unavailable", 503)
			return
		}
	}
	if runtime, ok := h.Audit.(*audit.Runtime); ok {
		ctx = audit.WithOperation(ctx, runtime, ev)
	}
	callStarted := time.Now()
	result, err := h.callResilient(ctx, sess, up, vt, args)
	callDuration := time.Since(callStarted)
	if err != nil {
		if errors.Is(err, errAuthorizationChanged) {
			_ = finish("denied", "authorization changed before dispatch")
			return
		}
		if errors.Is(err, errCircuitOpen) {
			if !finish("error", "circuit open") {
				http.Error(w, "audit outcome unavailable; operation may have completed", 503)
				return
			}
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
				if len(masked.Data) > 0 {
					data, redactions, privacyErr := vs.PII.MaskJSON(masked.Data)
					n += redactions
					if privacyErr != nil {
						masked.Data = nil
						masked.Message = "upstream error data denied by privacy policy"
					} else {
						masked.Data = data
					}
				}
				rpcErr, ev.Redactions = &masked, ev.Redactions+n
			}
			responsePayload, _ = json.Marshal(map[string]any{"error": rpcErr})
			if !finish("error", "upstream rpc error") {
				http.Error(w, "audit outcome unavailable; operation may have completed", 503)
				return
			}
			writeMessage(w, http.StatusOK, &mcp.Message{JSONRPC: mcp.JSONRPCVersion, ID: msg.ID, Error: rpcErr})
			return
		}
		if !finish("error", "upstream unavailable") {
			http.Error(w, "audit outcome unavailable; operation may have completed", 503)
			return
		}
		reason, detail := netpolicy.Diagnostic(err)
		stage := "upstream_call"
		var restErr *restapi.ExecutionError
		if errors.As(err, &restErr) {
			stage = restErr.Stage
			if stage == "tls_configuration" {
				reason, detail = "tls_configuration_invalid", "Check the upstream CA and client certificate configuration."
			}
		}
		h.Log.Error("upstream call failed", "upstream", up.Slug, "tool", vt.UpstreamName,
			"category", "transport_failure", "reason", reason, "detail", detail, "stage", stage,
			"virtual_server", vs.Slug, "request_id", ev.RequestID, "trace_id", ev.TraceID,
			"duration_ms", callDuration.Milliseconds())
		writeMessage(w, http.StatusOK, mcp.NewErrorResponse(msg.ID, mcp.CodeInternalError, "upstream unavailable"))
		return
	}
	if vs.PII != nil {
		masked, n, err := vs.PII.MaskJSON(result)
		if err != nil {
			// Never forward a result we could not inspect.
			if !finish("error", "result could not be masked") {
				http.Error(w, "audit outcome unavailable; operation may have completed", 503)
				return
			}
			writeMessage(w, http.StatusOK, mcp.NewErrorResponse(msg.ID, mcp.CodeInternalError, "upstream result could not be processed"))
			return
		}
		result, ev.Redactions = masked, ev.Redactions+n
	}
	var res struct {
		IsError bool `json:"isError"`
	}
	_ = json.Unmarshal(result, &res)
	ev.RawPayload = result
	if res.IsError {
		if !finish("tool_error", "") {
			http.Error(w, "audit outcome unavailable; operation may have completed", 503)
			return
		}
	} else {
		if !finish("ok", "") {
			http.Error(w, "audit outcome unavailable; operation may have completed", 503)
			return
		}
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
	if up.REST == nil && idempotent(vt.Tool.Annotations) {
		attempts = 2
		ctx = oauthtoken.WithReplaySafe(ctx)
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
		if err := checkAuthorization(ctx); err != nil {
			return nil, err
		}
		if br != nil && !br.Allow() {
			return nil, errCircuitOpen
		}
		if up.REST != nil {
			done := h.Metrics.BeginOperation(telemetry.Operation{Family: "upstream_requests", Upstream: up.Slug, Method: mcp.MethodToolsCall, Tool: vt.UpstreamName})
			res, err = up.REST.Execute(ctx, up.URL, vt.UpstreamName, args, up.Headers, up.TLS, restapi.Identity{Type: sess.owner.Mode, ID: sess.owner.CredentialID, Issuer: sess.owner.Issuer, Subject: sess.owner.Subject, Tenant: up.Tenant, VirtualServer: sess.slug}, func() error { return checkAuthorization(ctx) })
			done(operationOutcome(ctx, res, err))
		} else {
			res, err = h.callUpstream(ctx, sess, up, vt.UpstreamName, args)
		}
		if errors.Is(err, errAuthorizationChanged) {
			return res, err
		}
		if errors.Is(err, audit.ErrUnavailable) {
			return res, err
		}
		switch resilience.Classify(ctx, err) {
		case resilience.Reached:
			if br != nil {
				br.Success()
			}
			return res, err
		case resilience.Ignore:
			return res, err
		}

		if br != nil {
			br.Failure()
		}
	}
	return res, err
}

// callUpstream calls the tool, re-establishing the upstream session once if it expired.
func (h *Handler) callUpstream(ctx context.Context, sess *Session, up config.Upstream, tool string, args json.RawMessage) (result json.RawMessage, callErr error) {
	done := h.Metrics.BeginOperation(telemetry.Operation{Family: "upstream_requests", Upstream: up.Slug, Method: mcp.MethodToolsCall, Tool: tool})
	defer func() { done(operationOutcome(ctx, result, callErr)) }()
	for attempt := 0; ; attempt++ {
		if err := checkAuthorization(ctx); err != nil {
			return nil, err
		}
		c, release, err := sess.client(ctx, up)
		if err != nil {
			return nil, err
		}
		if err := checkAuthorization(ctx); err != nil {
			sess.reset(up.ID, c)
			release()
			return nil, err
		}
		h.notifyRoots(ctx, sess)
		if err := h.configureLogging(ctx, sess, c); err != nil {
			release()
			return nil, err
		}
		ctx = audit.WithUpstream(upstream.WithOrigin(ctx, up.Slug), up.Slug)
		res, err := c.CallTool(ctx, tool, args)
		release()
		if errors.Is(err, upstream.ErrSessionExpired) {
			sess.reset(up.ID, c)
			if attempt == 0 && oauthtoken.ReplaySafe(ctx) {
				continue
			}
		}
		return res, err
	}
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	snap, fresh := h.requestSnapshot(w, r)
	if !fresh {
		return
	}
	vs, ok := snap.VirtualServers[r.PathValue("slug")]
	if !ok {
		http.NotFound(w, r)
		return
	}
	owner, ok := h.authenticate(w, r, vs, snap)
	if !ok {
		return
	}
	sess, err := h.Sessions.Resolve(r.Context(), r.Header.Get("Mcp-Session-Id"))
	if err != nil {
		http.Error(w, "session store unavailable", http.StatusServiceUnavailable)
		return
	}
	if sess == nil || sess.slug != r.PathValue("slug") || sess.owner != owner {
		http.Error(w, "unknown session", http.StatusNotFound)
		return
	}
	if !validSessionVersion(w, r, sess) {
		return
	}
	if err := h.Sessions.End(r.Context(), sess.id); err != nil {
		http.Error(w, "session store unavailable", http.StatusServiceUnavailable)
		return
	}
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
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(10 * time.Second))
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(m)
}

// A known session supplies the version when older clients omit the header.
func validSessionVersion(w http.ResponseWriter, r *http.Request, sess *Session) bool {
	version := r.Header.Get("MCP-Protocol-Version")
	if !mcp.SupportsVersion(sess.version) || (version != "" && version != sess.version) {
		http.Error(w, "unsupported or mismatched MCP-Protocol-Version", http.StatusBadRequest)
		return false
	}
	return true
}

// requestSnapshot never admits a request from an unconfirmed configuration.
func (h *Handler) requestSnapshot(w http.ResponseWriter, r *http.Request) (*config.Snapshot, bool) {
	snap, err := h.Snap.AuthorizedSnapshot()
	if err != nil || snap == nil {
		http.Error(w, "authorization configuration unavailable", 503)
		return nil, false
	}
	return snap, true
}

type authorizationGuardKey struct{}

var errAuthorizationChanged = errors.New("authorization changed before dispatch")

func checkAuthorization(ctx context.Context) error {
	if check, ok := ctx.Value(authorizationGuardKey{}).(func(context.Context) error); ok {
		return check(ctx)
	}
	return nil
}

// Admission counters are removed on release, so arbitrary identities cannot grow the map.
func (h *Handler) acquire(owner Owner, server, clientIP string) (func(), bool) {
	h.budgetMu.Lock()
	defer h.budgetMu.Unlock()
	if h.budgets == nil {
		h.budgets = map[string]int{}
	}
	principalKey := "principal:" + owner.Mode + ":" + owner.Principal + ":" + owner.Subject
	if owner.Mode == "anonymous" && clientIP != "" {
		principalKey = "principal:anonymous:" + clientIP
	}
	keys := []string{"all", "server:" + server, principalKey}
	limits := []int{1024, 128, 16}
	for i, key := range keys {
		if h.budgets[key] >= limits[i] {
			return nil, false
		}
	}
	for _, key := range keys {
		h.budgets[key]++
	}
	return func() {
		h.budgetMu.Lock()
		defer h.budgetMu.Unlock()
		for _, key := range keys {
			h.budgets[key]--
			if h.budgets[key] == 0 {
				delete(h.budgets, key)
			}
		}
	}, true
}
