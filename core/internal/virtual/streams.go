package virtual

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/bishoku/vmcp/core/internal/audit"
	"github.com/bishoku/vmcp/core/internal/config"
	"github.com/bishoku/vmcp/core/internal/delivery"
	"github.com/bishoku/vmcp/core/internal/mcp"
	"github.com/bishoku/vmcp/core/internal/upstream"
)

func (h *Handler) streamCall(requestCtx context.Context, w http.ResponseWriter, r *http.Request, msg *mcp.Message, vs config.VirtualServer, sess *Session, snap *config.Snapshot) {
	var parameters mcp.CallToolParams
	if err := json.Unmarshal(msg.Params, &parameters); err != nil {
		writeMessage(w, 200, mcp.NewErrorResponse(msg.ID, mcp.CodeInvalidParams, "invalid call parameters"))
		return
	}
	if len(parameters.Meta) > 0 {
		var metadata map[string]json.RawMessage
		if json.Unmarshal(parameters.Meta, &metadata) != nil || metadata == nil {
			writeMessage(w, 200, mcp.NewErrorResponse(msg.ID, mcp.CodeInvalidParams, "_meta must be an object"))
			return
		}
		if raw, exists := metadata["progressToken"]; exists {
			var token any
			if len(raw) > 1024 || json.Unmarshal(raw, &token) != nil {
				writeMessage(w, 200, mcp.NewErrorResponse(msg.ID, mcp.CodeInvalidParams, "invalid progress token"))
				return
			}
			switch token.(type) {
			case string, float64:
			default:
				writeMessage(w, 200, mcp.NewErrorResponse(msg.ID, mcp.CodeInvalidParams, "invalid progress token"))
				return
			}
		}
	}
	h.workerMu.Lock()
	if h.workerStopped {
		h.workerMu.Unlock()
		http.Error(w, "server draining", 503)
		return
	}
	h.workerWG.Add(1)
	h.workerMu.Unlock()
	launched := false
	defer func() {
		if !launched {
			h.workerWG.Done()
		}
	}()
	principal, _ := json.Marshal(sess.owner)
	hash := sha256.Sum256(principal)
	op, err := h.Delivery.Begin(r.Context(), sess.id, msg.ID.String(), vs.Slug, hash[:])
	if err != nil {
		http.Error(w, "request delivery capacity unavailable", 503)
		return
	}
	// HTTP disconnection does not cancel admitted MCP work. Explicit cancellation and
	// the tool deadline do; audit completion retains its independent write allowance.
	ctx, cancel := context.WithTimeout(context.WithValue(context.WithoutCancel(requestCtx), deliveryRequestKey{}, op.ID), 70*time.Second)
	h.workerOnce.Do(func() { h.workerCtx, h.workerCancel = context.WithCancel(context.Background()) })
	launched = true
	go func() {
		defer h.workerWG.Done()
		stopWorker := context.AfterFunc(h.workerCtx, cancel)
		defer stopWorker()
		defer cancel()
		heartbeatDone := make(chan struct{})
		go func() {
			defer close(heartbeatDone)
			tick := time.NewTicker(time.Second)
			defer tick.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-tick.C:
					renewCtx, renewCancel := context.WithTimeout(ctx, time.Second)
					cancelled, err := h.Delivery.Renew(renewCtx, op)
					renewCancel()
					if err != nil || cancelled {
						cancel()
						return
					}
				}
			}
		}()
		recorder := newCapture()
		guarded := h.authorizationContext(ctx, recorder, r.WithContext(ctx), snap, vs, sess.owner)
		var incoming mcp.CallToolParams
		_ = json.Unmarshal(msg.Params, &incoming)
		var meta map[string]json.RawMessage
		_ = json.Unmarshal(incoming.Meta, &meta)
		originalToken := meta["progressToken"]
		upstreamToken := delivery.UUID()
		outgoingMeta, _ := json.Marshal(map[string]string{"progressToken": upstreamToken})
		if len(originalToken) == 0 {
			outgoingMeta = nil
		}
		var serverIDs sync.Map
		relay := upstream.Relay{Meta: outgoingMeta, OwnsRequest: func(id mcp.ID) bool { _, exists := serverIDs.Load(id.String()); return exists }}
		relay.Message = func(messageCtx context.Context, message *mcp.Message) (*mcp.Message, error) {
			switch message.Method {
			case mcp.MethodProgress:
				var params map[string]json.RawMessage
				if json.Unmarshal(message.Params, &params) != nil {
					return nil, errors.New("invalid progress")
				}
				var progress float64
				if json.Unmarshal(params["progress"], &progress) != nil || progress < 0 {
					return nil, errors.New("invalid progress value")
				}
				var token string
				_ = json.Unmarshal(params["progressToken"], &token)
				if len(originalToken) == 0 || token != upstreamToken {
					return nil, nil
				}
				params["progressToken"] = originalToken
				payload, _ := json.Marshal(params)
				if vs.PII != nil {
					masked, _, err := vs.PII.MaskJSON(payload)
					if err != nil {
						return nil, err
					}
					payload = masked
					var safe map[string]json.RawMessage
					_ = json.Unmarshal(payload, &safe)
					safe["progressToken"] = originalToken
					payload, _ = json.Marshal(safe)
				}
				output := *message
				output.Params = payload
				raw, _ := json.Marshal(output)
				if err := h.auditDelivery(messageCtx, sess, op.ID, "mcp.progress"); err != nil {
					return nil, err
				}
				return nil, h.Delivery.Append(messageCtx, op, raw, false)
			case mcp.MethodCancelled:
				var params struct {
					RequestID mcp.ID `json:"requestId"`
				}
				if json.Unmarshal(message.Params, &params) != nil {
					return nil, errors.New("invalid upstream cancellation")
				}
				value, ok := serverIDs.Load(params.RequestID.String())
				if !ok {
					return nil, nil
				}
				mapped := value.(string)
				notification, _ := mcp.NewRequest(mcp.ID{}, mcp.MethodCancelled, map[string]any{"requestId": mcp.NewStringID(mapped)})
				raw, _ := json.Marshal(notification)
				if err := h.auditDelivery(messageCtx, sess, op.ID, "mcp.server_cancellation"); err != nil {
					return nil, err
				}
				if err := h.Delivery.Append(messageCtx, op, raw, false); err != nil {
					return nil, err
				}
				response := mcp.NewErrorResponse(mcp.NewStringID(mapped), -32800, "request cancelled by upstream")
				raw, _ = json.Marshal(response)
				return nil, h.Delivery.CancelPending(messageCtx, op, mapped, raw)
			case mcp.MethodToolsListChanged, "notifications/resources/list_changed", "notifications/prompts/list_changed":
				if _, err := h.Delivery.Pool.Exec(messageCtx, `INSERT INTO mcp_discovery_requests(upstream_id) SELECT DISTINCT upstream_id FROM (SELECT upstream_id,virtual_server_id FROM virtual_server_tools UNION SELECT upstream_id,virtual_server_id FROM virtual_catalog) vt JOIN virtual_servers v ON v.id=vt.virtual_server_id WHERE v.slug=$1 ON CONFLICT(upstream_id) DO UPDATE SET requested_at=now()`, vs.Slug); err != nil {
					return nil, err
				}
				return nil, nil
			case mcp.MethodPing:
				if !message.IsRequest() {
					return nil, nil
				}
				mapped := delivery.UUID()
				serverIDs.Store(message.ID.String(), mapped)
				defer serverIDs.Delete(message.ID.String())
				if err := h.Delivery.Request(messageCtx, op, mapped); err != nil {
					return nil, err
				}
				forwarded := *message
				forwarded.ID = mcp.NewStringID(mapped)
				forwarded.Params = nil
				raw, _ := json.Marshal(forwarded)
				if err := h.auditDelivery(messageCtx, sess, op.ID, "mcp.server_request"); err != nil {
					return nil, err
				}
				if err := h.Delivery.Append(messageCtx, op, raw, false); err != nil {
					return nil, err
				}
				reply, err := h.Delivery.WaitReply(messageCtx, op, mapped)
				if err != nil {
					return nil, err
				}
				response, err := mcp.Decode(reply)
				if err != nil {
					return nil, err
				}
				response.ID = message.ID
				return response, nil
			default:
				if message.IsRequest() {
					return mcp.NewErrorResponse(message.ID, mcp.CodeMethodNotFound, "client feature not negotiated or permitted"), nil
				}
				return nil, nil
			}
		}
		guarded = upstream.WithRelay(guarded, relay)
		if msg.Method == mcp.MethodToolsCall {
			h.callTool(guarded, recorder, msg, vs, sess, snap, h.ipString(r))
		} else {
			h.catalogCall(guarded, recorder, msg, vs, sess, snap, h.ipString(r))
		}
		response, err := mcp.Decode(recorder.Body.Bytes())
		if err != nil {
			response = mcp.NewErrorResponse(msg.ID, mcp.CodeInternalError, "operation failed; consult correlated audit evidence")
		}
		raw, _ := json.Marshal(response)
		finishCtx, finishCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer finishCancel()
		if err := h.Delivery.Append(finishCtx, op, raw, true); err != nil && h.Log != nil {
			h.Log.Error("MCP completion delivery unavailable", "operation_id", op.ID)
		}
		cancel()
		<-heartbeatDone
	}()
	w.Header().Set("X-Request-Id", op.ID)
	h.serveStream(w, r, sess, op.Stream, 0, true)
}

func (h *Handler) authorizationContext(ctx context.Context, w http.ResponseWriter, r *http.Request, snap *config.Snapshot, vs config.VirtualServer, owner Owner) context.Context {
	return context.WithValue(ctx, authorizationGuardKey{}, func(dispatchCtx context.Context) error {
		request := r.WithContext(dispatchCtx)
		current, ok := h.requestSnapshot(w, request)
		if !ok {
			return errAuthorizationChanged
		}
		if current.Revision != snap.Revision {
			http.Error(w, "configuration changed; retry", 409)
			return errAuthorizationChanged
		}
		verified, ok := h.authenticate(w, request, vs, current)
		if !ok || verified != owner {
			return errAuthorizationChanged
		}
		live, err := h.Sessions.Resolve(dispatchCtx, r.Header.Get("Mcp-Session-Id"))
		if err != nil || live == nil || live.owner != owner {
			http.Error(w, "session unavailable", 503)
			return errAuthorizationChanged
		}
		return nil
	})
}

func (h *Handler) getStream(w http.ResponseWriter, r *http.Request) {
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
	release, admitted := h.acquire(owner, vs.Slug)
	if !admitted {
		http.Error(w, "stream concurrency limit exceeded", 429)
		return
	}
	defer release()
	if r.Header.Get("Mcp-Session-Id") == "" {
		http.Error(w, "missing Mcp-Session-Id header", 400)
		return
	}
	sess, err := h.Sessions.Resolve(r.Context(), r.Header.Get("Mcp-Session-Id"))
	if err != nil {
		http.Error(w, "session unavailable", 503)
		return
	}
	if sess == nil || sess.slug != vs.Slug || sess.owner != owner {
		http.Error(w, "unknown session", 404)
		return
	}
	if !validSessionVersion(w, r, sess) {
		return
	}
	ready, err := h.Sessions.Ready(r.Context(), sess)
	if err != nil || !ready {
		http.Error(w, "session not initialized", 400)
		return
	}
	var stream string
	var seq int64
	if event := r.Header.Get("Last-Event-ID"); event != "" {
		stream, seq, err = h.Delivery.Resume(r.Context(), sess.id, event)
	} else {
		stream, err = h.Delivery.GetStream(r.Context(), sess.id)
	}
	if err != nil {
		http.Error(w, "unknown or expired replay cursor", 400)
		return
	}
	h.serveStream(w, r, sess, stream, seq, false)
}
func (h *Handler) serveStream(w http.ResponseWriter, r *http.Request, sess *Session, stream string, seq int64, post bool) {
	owner := delivery.UUID()
	if err := h.Delivery.Claim(r.Context(), sess.id, stream, owner); err != nil {
		http.Error(w, "stream already owned or unavailable", 409)
		return
	}
	h.activeStreams.Add(1)
	defer h.activeStreams.Add(-1)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		h.Delivery.Release(ctx, stream, owner)
	}()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	controller := http.NewResponseController(w)
	_ = controller.SetWriteDeadline(time.Now().Add(10 * time.Second))
	fmt.Fprint(w, ": connected\n\n")
	if controller.Flush() != nil {
		return
	}
	duration := 65 * time.Second
	if post {
		duration = 75 * time.Second
	}
	timeout := time.NewTimer(duration)
	defer timeout.Stop()
	wake, unsubscribe := h.Delivery.Subscribe(sess.id)
	defer unsubscribe()
	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()
	checked := time.Time{}
	for {
		if time.Since(checked) > time.Second {
			// Reauthenticate reconnects and active streams. Never write auth errors into SSE.
			checkCtx, checkCancel := context.WithTimeout(r.Context(), time.Second)
			defer checkCancel()
			checkRequest := r.WithContext(checkCtx)
			check := newCapture()
			snap, fresh := h.requestSnapshot(check, checkRequest)
			if !fresh {
				return
			}
			vs, exists := snap.VirtualServers[sess.slug]
			if !exists {
				return
			}
			verified, ok := h.authenticate(check, checkRequest, vs, snap)
			if !ok || verified != sess.owner {
				return
			}
			live, err := h.Sessions.Resolve(checkCtx, sess.id)
			if err != nil || live == nil || live.owner != sess.owner {
				return
			}
			if err := h.Delivery.Claim(checkCtx, sess.id, stream, owner); err != nil {
				return
			}
			checkCancel()
			checked = time.Now()
		}
		readCtx, readCancel := context.WithTimeout(r.Context(), time.Second)
		events, err := h.Delivery.Events(readCtx, sess.id, stream, owner, seq)
		readCancel()
		if err != nil {
			return
		}
		for _, event := range events {
			fenceCtx, fenceCancel := context.WithTimeout(r.Context(), time.Second)
			claimErr := h.Delivery.Claim(fenceCtx, sess.id, stream, owner)
			fenceCancel()
			if claimErr != nil {
				return
			}
			_ = controller.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if _, err := fmt.Fprintf(w, "id: %s\nevent: message\ndata: %s\n\n", event.ID, strings.TrimSpace(string(event.Payload))); err != nil {
				return
			}
			if controller.Flush() != nil {
				return
			}
			seq = event.Seq
			if event.Terminal {
				return
			}
		}
		select {
		case <-r.Context().Done():
			return
		case <-timeout.C:
			return
		case <-tick.C:
		case <-wake:
		}
	}
}

func (h *Handler) deliveryInput(w http.ResponseWriter, r *http.Request, sess *Session, msg *mcp.Message, vs config.VirtualServer) bool {
	if msg.IsResponse() {
		var id string
		if json.Unmarshal([]byte(msg.ID.String()), &id) != nil {
			http.Error(w, "unknown pending request", 400)
			return true
		}
		copy := *msg
		if vs.PII != nil {
			if copy.Result != nil {
				masked, _, err := vs.PII.MaskJSON(copy.Result)
				if err != nil {
					http.Error(w, "client response cannot be inspected", 400)
					return true
				}
				copy.Result = masked
			}
			if copy.Error != nil {
				masked := *copy.Error
				masked.Message, _ = vs.PII.MaskString(masked.Message)
				if len(masked.Data) > 0 {
					data, _, err := vs.PII.MaskJSON(masked.Data)
					if err != nil {
						http.Error(w, "client error cannot be inspected", 400)
						return true
					}
					masked.Data = data
				}
				copy.Error = &masked
			}
		}
		raw, _ := json.Marshal(copy)
		if err := h.auditDelivery(r.Context(), sess, "", "mcp.client_response"); err != nil {
			http.Error(w, "audit unavailable", 503)
			return true
		}
		if err := h.Delivery.Reply(r.Context(), sess.id, id, raw); err != nil {
			http.Error(w, "unknown pending request", 400)
		} else {
			w.WriteHeader(202)
		}
		return true
	}
	if msg.IsNotification() && msg.Method == mcp.MethodCancelled {
		var p struct {
			RequestID mcp.ID `json:"requestId"`
		}
		if json.Unmarshal(msg.Params, &p) != nil || p.RequestID.IsZero() {
			http.Error(w, "invalid cancellation", 400)
			return true
		}
		if err := h.auditDelivery(r.Context(), sess, "", "mcp.cancellation"); err != nil {
			http.Error(w, "audit unavailable", 503)
			return true
		}
		if err := h.Delivery.Cancel(r.Context(), sess.id, p.RequestID.String()); err != nil {
			http.Error(w, "cancellation unavailable", 503)
		} else {
			w.WriteHeader(202)
		}
		return true
	}
	return false
}

func (h *Handler) auditDelivery(ctx context.Context, sess *Session, operation, action string) error {
	return audit.Persist(ctx, h.Delivery.Pool, audit.Event{Time: time.Now().UTC(), VirtualServer: sess.slug, AgentKey: sess.keySlug, Subject: sess.owner.Subject, ActorType: sess.owner.Mode, ActorID: sess.owner.CredentialID, Issuer: sess.owner.Issuer, Action: action, Method: action, Target: sess.slug, OperationID: operation, RequestID: operation, Status: "ok", Stage: "event", TraceID: audit.TraceFrom(ctx)})
}
func (h *Handler) WaitWorkers(ctx context.Context) error {
	done := make(chan struct{})
	go func() { h.workerWG.Wait(); close(done) }()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (h *Handler) StopWorkers() {
	h.workerMu.Lock()
	defer h.workerMu.Unlock()
	h.workerStopped = true
	h.workerOnce.Do(func() { h.workerCtx, h.workerCancel = context.WithCancel(context.Background()) })
	h.workerCancel()
}

func (h *Handler) ActiveStreams() int64 { return h.activeStreams.Load() }

type deliveryRequestKey struct{}
type responseCapture struct {
	header http.Header
	Body   bytes.Buffer
	status int
}

func newCapture() *responseCapture             { return &responseCapture{header: http.Header{}} }
func (c *responseCapture) Header() http.Header { return c.header }
func (c *responseCapture) WriteHeader(status int) {
	if c.status == 0 {
		c.status = status
	}
}
func (c *responseCapture) Write(data []byte) (int, error) {
	if c.Body.Len()+len(data) > 12<<20 {
		return 0, delivery.ErrCapacity
	}
	if c.status == 0 {
		c.status = 200
	}
	return c.Body.Write(data)
}
