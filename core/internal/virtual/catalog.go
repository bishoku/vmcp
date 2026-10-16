package virtual

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/bishoku/vmcp/core/internal/audit"
	"github.com/bishoku/vmcp/core/internal/config"
	"github.com/bishoku/vmcp/core/internal/mcp"
	"github.com/bishoku/vmcp/core/internal/telemetry"
	"github.com/bishoku/vmcp/core/internal/upstream"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func catalogRoute(vs config.VirtualServer, kind, exposed string, expand bool) (config.VirtualCatalogObject, string, bool) {
	return vs.CatalogRoute(kind, exposed, expand)
}
func (h *Handler) listCatalog(w http.ResponseWriter, msg *mcp.Message, vs config.VirtualServer, revision int64, kind, field string) {
	if (kind == "prompt" && vs.CatalogCapabilities().Prompts == nil) || (kind != "prompt" && vs.CatalogCapabilities().Resources == nil) {
		writeMessage(w, 200, mcp.NewErrorResponse(msg.ID, mcp.CodeMethodNotFound, "catalog capability unavailable"))
		return
	}
	var p struct {
		Cursor string `json:"cursor"`
	}
	if len(msg.Params) > 0 && json.Unmarshal(msg.Params, &p) != nil {
		writeMessage(w, 200, mcp.NewErrorResponse(msg.ID, mcp.CodeInvalidParams, "invalid pagination"))
		return
	}
	offset := 0
	if p.Cursor != "" {
		raw, err := base64.RawURLEncoding.DecodeString(p.Cursor)
		parts := strings.Split(string(raw), ":")
		if err != nil || len(parts) != 3 || parts[0] != kind || parts[1] != strconv.FormatInt(revision, 10) {
			writeMessage(w, 200, mcp.NewErrorResponse(msg.ID, mcp.CodeInvalidParams, "invalid or stale cursor"))
			return
		}
		offset, err = strconv.Atoi(parts[2])
		if err != nil || offset < 0 {
			writeMessage(w, 200, mcp.NewErrorResponse(msg.ID, mcp.CodeInvalidParams, "invalid cursor"))
			return
		}
	}
	objects, err := vs.ListedCatalog(kind)
	if err != nil {
		writeMessage(w, 200, mcp.NewErrorResponse(msg.ID, mcp.CodeInternalError, "catalog metadata cannot be inspected"))
		return
	}
	if offset > len(objects) {
		writeMessage(w, 200, mcp.NewErrorResponse(msg.ID, mcp.CodeInvalidParams, "invalid cursor"))
		return
	}
	end := min(offset+200, len(objects))
	result := map[string]any{field: objects[offset:end]}
	if end < len(objects) {
		result["nextCursor"] = base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%d:%d", kind, revision, end)))
	}
	h.reply(w, msg.ID, result)
}
func (h *Handler) listTools(w http.ResponseWriter, msg *mcp.Message, vs config.VirtualServer, revision int64) {
	var p mcp.ListToolsParams
	if len(msg.Params) > 0 && json.Unmarshal(msg.Params, &p) != nil {
		writeMessage(w, 200, mcp.NewErrorResponse(msg.ID, mcp.CodeInvalidParams, "invalid pagination"))
		return
	}
	offset := 0
	if p.Cursor != "" {
		raw, err := base64.RawURLEncoding.DecodeString(p.Cursor)
		parts := strings.Split(string(raw), ":")
		if err != nil || len(parts) != 3 || parts[0] != "tool" || parts[1] != strconv.FormatInt(revision, 10) {
			writeMessage(w, 200, mcp.NewErrorResponse(msg.ID, mcp.CodeInvalidParams, "invalid or stale cursor"))
			return
		}
		offset, err = strconv.Atoi(parts[2])
		if err != nil || offset < 0 {
			writeMessage(w, 200, mcp.NewErrorResponse(msg.ID, mcp.CodeInvalidParams, "invalid cursor"))
			return
		}
	}
	if offset > len(vs.Tools) {
		writeMessage(w, 200, mcp.NewErrorResponse(msg.ID, mcp.CodeInvalidParams, "invalid cursor"))
		return
	}
	end := min(offset+200, len(vs.Tools))
	tools, err := vs.ListedTools(offset, end)
	if err != nil {
		writeMessage(w, 200, mcp.NewErrorResponse(msg.ID, mcp.CodeInternalError, "tool metadata cannot be inspected"))
		return
	}
	res := mcp.ListToolsResult{Tools: tools}
	if end < len(vs.Tools) {
		res.NextCursor = base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf("tool:%d:%d", revision, end)))
	}
	h.reply(w, msg.ID, res)
}
func validatePromptArguments(o config.VirtualCatalogObject, p map[string]json.RawMessage) error {
	var args map[string]string
	if p["arguments"] != nil && (json.Unmarshal(p["arguments"], &args) != nil || args == nil) {
		return errors.New("prompt arguments must be strings")
	}
	var defs []struct {
		Name     string `json:"name"`
		Required bool   `json:"required"`
	}
	if raw := o.Definition["arguments"]; len(raw) > 0 && json.Unmarshal(raw, &defs) != nil {
		return errors.New("invalid prompt definition")
	}
	allowed := map[string]bool{}
	for _, d := range defs {
		allowed[d.Name] = true
		if _, ok := args[d.Name]; d.Required && !ok {
			return errors.New("required prompt argument is missing")
		}
	}
	for k, v := range args {
		if !allowed[k] || len(v) > 64<<10 {
			return errors.New("unknown or oversized prompt argument")
		}
	}
	return nil
}
func inspectCatalogResult(raw json.RawMessage, vs config.VirtualServer) (json.RawMessage, error) {
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return nil, errors.New("invalid result")
	}
	var inspect func(any, int) error
	inspect = func(v any, depth int) error {
		if depth > 64 {
			return errors.New("content nesting exceeded")
		}
		switch x := v.(type) {
		case map[string]any:
			if vs.PII != nil {
				if _, ok := x["blob"]; ok {
					return errors.New("binary resource is not inspectable")
				}
				switch x["type"] {
				case "image", "audio":
					return errors.New("binary content is not inspectable")
				}
			}
			for _, child := range x {
				if err := inspect(child, depth+1); err != nil {
					return err
				}
			}
		case []any:
			for _, child := range x {
				if err := inspect(child, depth+1); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := inspect(value, 0); err != nil {
		return nil, err
	}
	if vs.PII != nil {
		masked, _, err := vs.PII.MaskJSON(raw)
		return masked, err
	}
	return raw, nil
}
func (h *Handler) catalogCall(ctx context.Context, w http.ResponseWriter, msg *mcp.Message, vs config.VirtualServer, sess *Session, snap *config.Snapshot, ip string) {
	fail := func(code int, message string) { writeMessage(w, 200, mcp.NewErrorResponse(msg.ID, code, message)) }
	var params map[string]json.RawMessage
	if json.Unmarshal(msg.Params, &params) != nil || params == nil {
		fail(mcp.CodeInvalidParams, "parameters must be an object")
		return
	}
	kind, selector := "resource", "uri"
	if msg.Method == "prompts/get" {
		kind, selector = "prompt", "name"
	}
	completion := msg.Method == "completion/complete"
	var ref map[string]json.RawMessage
	if completion {
		if json.Unmarshal(params["ref"], &ref) != nil {
			fail(mcp.CodeInvalidParams, "invalid completion reference")
			return
		}
		var typ string
		_ = json.Unmarshal(ref["type"], &typ)
		switch typ {
		case "ref/prompt":
			kind, selector = "prompt", "name"
		case "ref/resource":
			kind, selector = "template", "uri"
		default:
			fail(mcp.CodeInvalidParams, "unsupported completion reference")
			return
		}
	}
	var exposed string
	identity := params[selector]
	if completion {
		identity = ref[selector]
	}
	if json.Unmarshal(identity, &exposed) != nil || exposed == "" {
		fail(mcp.CodeInvalidParams, "missing catalog identity")
		return
	}
	object, original, ok := catalogRoute(vs, kind, exposed, !completion)
	if !ok {
		fail(-32002, "catalog object not found or not permitted")
		return
	}
	if msg.Method == "prompts/get" {
		if err := validatePromptArguments(object, params); err != nil {
			fail(mcp.CodeInvalidParams, err.Error())
			return
		}
	}
	up, ok := snap.Upstreams[object.UpstreamID]
	if !ok {
		fail(mcp.CodeInternalError, "upstream unavailable")
		return
	}
	if _, limited := h.rateLimited(ctx, vs, snap, sess.keySlug); limited {
		fail(codeRateLimited, "rate limit exceeded")
		return
	}
	if completion && !object.Complete {
		fail(mcp.CodeMethodNotFound, "completion unavailable")
		return
	}
	if completion {
		var arg struct {
			Name  string `json:"name"`
			Value string `json:"value"`
		}
		if json.Unmarshal(params["argument"], &arg) != nil || arg.Name == "" || len(arg.Value) > 64<<10 {
			fail(mcp.CodeInvalidParams, "invalid completion argument")
			return
		}
		ref[selector], _ = json.Marshal(original)
		params["ref"], _ = json.Marshal(ref)
	} else {
		params[selector], _ = json.Marshal(original)
	}
	if vs.PII != nil && vs.PII.MaskArguments {
		for _, key := range []string{"arguments", "argument", "context"} {
			if len(params[key]) > 0 {
				masked, _, err := vs.PII.MaskJSON(params[key])
				if err != nil {
					fail(mcp.CodeInvalidParams, "arguments cannot be inspected")
					return
				}
				params[key] = masked
			}
		}
	}
	raw, _ := json.Marshal(params)
	if err := h.Approval.Check(ctx, PolicyDecision{Actor: sess.owner.CredentialID, Server: vs.Slug, Method: msg.Method, Object: exposed, Revision: snap.Revision}, raw); err != nil {
		h.record(audit.Event{Time: time.Now().UTC(), Action: msg.Method, Method: msg.Method, Target: vs.Slug, Status: "denied", Reason: "approval policy", ActorType: sess.owner.Mode, ActorID: sess.owner.CredentialID, Revision: snap.Revision})
		fail(-32003, "approval policy denied operation")
		return
	}
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	ev := audit.Event{Time: time.Now().UTC(), VirtualServer: vs.Slug, AgentKey: sess.keySlug, ClientIP: ip, Subject: subjectFrom(ctx), Method: msg.Method, ActorType: sess.owner.Mode, ActorID: sess.owner.CredentialID, Issuer: sess.owner.Issuer, Revision: snap.Revision, Action: msg.Method, Target: vs.Slug, Upstream: up.Slug, TraceID: audit.TraceFrom(ctx)}
	if id, ok := ctx.Value(deliveryRequestKey{}).(string); ok {
		ev.RequestID = id
	}
	durable, strict := h.Audit.(audit.DurableRecorder)
	var err error
	if strict {
		ev, err = durable.Admit(ctx, ev)
		if err != nil {
			fail(mcp.CodeInternalError, "audit admission unavailable")
			return
		}
	}
	var result json.RawMessage
	if msg.Method == "resources/subscribe" || msg.Method == "resources/unsubscribe" {
		err = h.subscription(ctx, sess, object, exposed, original, snap.Revision, msg.Method == "resources/subscribe")
		result = json.RawMessage(`{}`)
	} else {
		result, err = h.callCatalogUpstream(ctx, sess, up, msg.Method, raw)
	}
	if err == nil {
		result, err = inspectCatalogResult(result, vs)
	}
	if err == nil && msg.Method == "resources/read" {
		var body map[string]json.RawMessage
		var contents []map[string]json.RawMessage
		if json.Unmarshal(result, &body) != nil || json.Unmarshal(body["contents"], &contents) != nil || contents == nil {
			err = errors.New("invalid resource response")
		} else {
			for _, c := range contents {
				var uri string
				_ = json.Unmarshal(c["uri"], &uri)
				if uri != original {
					err = errors.New("unselected resource in response")
					break
				}
				c["uri"], _ = json.Marshal(exposed)
			}
			body["contents"], _ = json.Marshal(contents)
			result, _ = json.Marshal(body)
		}
	}
	ev.Status = "ok"
	if err != nil {
		ev.Status = "error"
		ev.Reason = "catalog operation failed"
	}
	if strict {
		finishCtx, done := context.WithTimeout(context.WithoutCancel(ctx), audit.WriteTimeout)
		finishErr := durable.Complete(finishCtx, ev)
		done()
		if finishErr != nil {
			fail(mcp.CodeInternalError, "audit outcome unavailable")
			return
		}
	} else {
		h.record(ev)
	}
	if err != nil {
		fail(mcp.CodeInternalError, "catalog operation failed or content cannot be inspected")
		return
	}
	writeMessage(w, 200, &mcp.Message{JSONRPC: mcp.JSONRPCVersion, ID: msg.ID, Result: result})
}
func (h *Handler) callCatalogUpstream(ctx context.Context, sess *Session, up config.Upstream, method string, params json.RawMessage) (result json.RawMessage, callErr error) {
	done := h.Metrics.BeginOperation(telemetry.Operation{Family: "upstream_requests", Upstream: up.Slug, Method: method})
	defer func() { done(operationOutcome(ctx, result, callErr)) }()
	if err := checkAuthorization(ctx); err != nil {
		return nil, err
	}
	c, release, err := sess.client(ctx, up)
	if err != nil {
		return nil, err
	}
	defer release()
	if err := checkAuthorization(ctx); err != nil {
		return nil, err
	}
	h.notifyRoots(ctx, sess)
	if err := h.configureLogging(ctx, sess, c); err != nil {
		return nil, err
	}
	ctx = upstream.WithOrigin(ctx, up.Slug)
	return upstream.Call(ctx, c, method, params)
}
