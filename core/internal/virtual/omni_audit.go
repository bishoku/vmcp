package virtual

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/bishoku/vmcp/core/internal/audit"
	"github.com/bishoku/vmcp/core/internal/config"
	"github.com/bishoku/vmcp/core/internal/mcp"
	"github.com/bishoku/vmcp/core/internal/omni"
)

type omniAuditParentKey struct{}

type omniAuditTicket struct {
	Event    audit.Event `json:"event"`
	Identity string      `json:"identity"`
	Expires  time.Time   `json:"expires"`
}

func omniAuditIdentity(r *http.Request, ip string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(r.Header.Get("Authorization")+"\x00"+ip)))
}
func (h *Handler) decodeOmniAudit(r *http.Request, token string) (audit.Event, bool) {
	runtime, ok := h.Audit.(*audit.Runtime)
	if !ok || runtime.Box == nil {
		return audit.Event{}, false
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return audit.Event{}, false
	}
	plain, err := runtime.Box.OpenBound(raw, "omni", "audit.operation")
	if err != nil {
		return audit.Event{}, false
	}
	var ticket omniAuditTicket
	if json.Unmarshal(plain, &ticket) != nil || ticket.Expires.Before(time.Now()) || subtle.ConstantTimeCompare([]byte(ticket.Identity), []byte(omniAuditIdentity(r, h.ipString(r)))) != 1 {
		return audit.Event{}, false
	}
	return ticket.Event, true
}
func (h *Handler) omniAuditBegin(w http.ResponseWriter, r *http.Request) {
	runtime, ok := h.Audit.(*audit.Runtime)
	if !ok || runtime.Box == nil {
		http.Error(w, "audit unavailable", 503)
		return
	}
	var p omni.AuditStart
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 5<<20)).Decode(&p) != nil {
		http.Error(w, "invalid audit request", 400)
		return
	}
	switch p.Tool {
	case "discover_server", "discover_tools", "get_tool_schema", "invoke_tool":
	default:
		http.Error(w, "invalid audit tool", 400)
		return
	}
	snap, ok := h.requestSnapshot(w, r)
	if !ok {
		return
	}
	owner := Owner{Mode: "unauthenticated"}
	auth := &Handler{JWT: h.JWT, PublicURL: h.PublicURL, Log: h.Log, TrustedProxies: h.TrustedProxies}
	if r.Header.Get("Authorization") == "" {
		owner.Mode = "anonymous"
	} else {
		if verified, valid := auth.authenticateRuntime(&discardResponse{header: http.Header{}}, r, config.VirtualServer{Slug: "__omni_auth_probe__"}, snap, false); valid {
			owner = verified
		}
	}
	policy := snap.Audit.ForOmni()
	e := audit.Event{Scope: "omni", Boundary: "omni", Action: "omni/" + p.Tool, Method: "tools/call", Tool: p.Tool, Target: "omni", ClientIP: h.ipString(r), ActorType: owner.Mode, ActorID: owner.CredentialID, AgentKey: owner.Principal, Issuer: owner.Issuer, Subject: owner.Subject, Revision: snap.Revision, TraceID: audit.TraceFrom(audit.WithTrace(r.Context(), r.Header.Get("traceparent"))), Policy: &policy, RawPayload: p.Payload}
	var clientCall struct {
		Params mcp.CallToolParams `json:"params"`
	}
	_ = json.Unmarshal(p.Payload, &clientCall)
	e.IDs = callerCorrelation(snap, owner, clientCall.Params.Meta)
	e, err := runtime.Admit(r.Context(), e)
	if err != nil {
		http.Error(w, "audit admission unavailable", 503)
		return
	}
	e.Ciphertext = nil
	e.RawPayload = nil
	encoded, err := json.Marshal(omniAuditTicket{Event: e, Identity: omniAuditIdentity(r, h.ipString(r)), Expires: time.Now().Add(10 * time.Minute)})
	if err != nil {
		http.Error(w, "audit unavailable", 503)
		return
	}
	sealed, err := runtime.Box.SealBound(encoded, "omni", "audit.operation")
	if err != nil {
		http.Error(w, "audit unavailable", 503)
		return
	}
	capture := policy.Scopes
	options := omni.AuditOptions{Details: capture["omni_diagnostics"].Response, Candidates: policy.Omni.Candidates, Denied: policy.Omni.IncludeDenied, EmbeddingRequest: capture["omni_embedding"].Request, EmbeddingResponse: capture["omni_embedding"].Response, RerankRequest: capture["omni_rerank"].Request, RerankResponse: capture["omni_rerank"].Response, Limit: policy.Omni.MaxCandidates}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(omni.AuditOperation{Ticket: base64.RawURLEncoding.EncodeToString(sealed), ID: e.OperationID, Options: options})
}
func (h *Handler) omniAuditComplete(w http.ResponseWriter, r *http.Request) {
	var p omni.AuditFinish
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 6<<20)).Decode(&p) != nil {
		http.Error(w, "invalid audit completion", 400)
		return
	}
	e, ok := h.decodeOmniAudit(r, p.Ticket)
	if !ok {
		http.Error(w, "invalid audit ticket", 403)
		return
	}
	runtime := h.Audit.(*audit.Runtime)
	if p.Status != "ok" && p.Status != "error" && p.Status != "tool_error" && p.Status != "denied" {
		http.Error(w, "invalid audit status", 400)
		return
	}
	// Do not require a still-valid runtime grant here: revoked/expired callers must
	// be able to finish the audit of an already-admitted operation, not invoke tools.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), audit.WriteTimeout)
	defer cancel()
	if p.Trace != nil {
		// Model bodies have independent capture switches and never piggyback on the
		// general diagnostics toggle. Only the private bridge can submit these events.
		for n := range p.Trace.Stages {
			stage := &p.Trace.Stages[n]
			var scope, direction string
			switch stage.Name {
			case "embedding_request":
				scope, direction = "omni_embedding", "request"
			case "embedding_response":
				scope, direction = "omni_embedding", "response"
			case "reranker_request":
				scope, direction = "omni_rerank", "request"
			case "reranker_response":
				scope, direction = "omni_rerank", "response"
			}
			if scope == "" {
				continue
			}
			provider := e
			provider.OperationID = audit.OutcomeID(scope + e.OperationID)
			provider.ParentOperationID = e.OperationID
			provider.EventID = audit.OutcomeID(fmt.Sprintf("%s:%s:%d:%s", scope, direction, n, e.OperationID))
			provider.Time = time.Now().UTC()
			provider.Scope = scope
			provider.Boundary = scope
			provider.Action = stage.Name
			provider.Method = stage.Name
			provider.Stage = "event"
			provider.Direction = direction
			provider.Status = "ok"
			provider.Reason = stage.Status
			if stage.Status != "ok" {
				provider.Status = "error"
			}
			provider.RawPayload, _ = json.Marshal(stage.Details)
			if stage.Status == "omitted_size" {
				provider.RawPayload = make([]byte, (2<<20)+1)
				provider.RawPayloadBytes = stage.OriginalBytes
			}
			if err := runtime.Write(ctx, provider); err != nil {
				http.Error(w, "provider audit unavailable", 503)
				return
			}
			stage.Details = nil
		}

		child := e
		child.OperationID = audit.OutcomeID("omni-diagnostics:" + e.OperationID)
		child.ParentOperationID = e.OperationID
		child.EventID = audit.OutcomeID("trace-event:" + e.OperationID)
		child.Time = time.Now().UTC()
		child.Scope = "omni_diagnostics"
		child.Boundary = "omni_discovery"
		child.Action = "omni/discovery_pipeline"
		child.Method = "discovery_pipeline"
		child.Status = p.Status
		child.DurationMS = p.DurationMS
		child.Direction = "response"
		child.Stage = "outcome"
		child.Reason = omniTraceSummary(p.Trace.Counts)
		child.RawPayload, _ = json.Marshal(p.Trace)
		if err := runtime.Write(ctx, child); err != nil {
			http.Error(w, "audit diagnostics unavailable", 503)
			return
		}
	}
	e.Status = p.Status
	e.Reason = p.Reason
	e.DurationMS = p.DurationMS
	e.RawPayload = p.Payload
	e.RawPayloadBytes = p.OriginalBytes
	if p.OriginalBytes > 2<<20 {
		e.RawPayload = make([]byte, (2<<20)+1)
	}
	if err := runtime.Complete(ctx, e); err != nil {
		http.Error(w, "audit outcome unavailable", 503)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}
func omniTraceSummary(counts map[string]int) string {
	keys := []string{}
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := []string{}
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d", key, counts[key]))
	}
	return strings.Join(parts, "; ")
}
