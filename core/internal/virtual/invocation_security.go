package virtual

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"time"

	"github.com/bishoku/vmcp/core/internal/audit"
	"github.com/bishoku/vmcp/core/internal/config"
	"github.com/bishoku/vmcp/core/internal/invocation"
	"github.com/bishoku/vmcp/core/internal/mcp"
)

type assuranceKey struct{}
type approvedInvocationKey struct{}
type retryInvocationKey struct{}

// InvocationOperationKey is client metadata, not an authorization credential.
const InvocationOperationKey = "io.vmcp/operationId"

func (h *Handler) enforceInvocation(ctx context.Context, w http.ResponseWriter, msg *mcp.Message, snap *config.Snapshot, vs config.VirtualServer, owner Owner, upstream, tool string, raw, meta json.RawMessage, executionContext ...any) bool {
	rule, err := snap.InvocationSecurity.Effective(upstream, tool)
	deny := func(status int, reason string, data any) bool {
		h.record(audit.Event{Access: snap.ConsumerAudit(vs), Time: time.Now().UTC(), Revision: snap.Revision, VirtualServer: vs.Slug, Method: msg.Method, Tool: tool, ActorType: owner.Mode, ActorID: owner.CredentialID, Issuer: owner.Issuer, Subject: owner.Subject, Status: "denied", Reason: reason})
		response := mcp.NewErrorResponse(msg.ID, -32004, reason)
		response.Error.Data, _ = json.Marshal(data)
		writeMessage(w, status, response)
		return false
	}
	if err != nil {
		return deny(403, "invalid invocation policy", nil)
	}
	id, _ := ctx.Value(assuranceKey{}).(invocation.Identity)
	if id.Mode == "" {
		id = invocation.Identity{Mode: owner.Mode, CredentialID: owner.CredentialID, Issuer: owner.Issuer, Subject: owner.Subject}
	}
	protected := rule.StepUp != nil || len(rule.ApprovalServices) > 0 || (rule.Identity != nil && rule.Identity.Source == "caller_context")
	if !protected {
		return true
	}
	if rule.Identity == nil {
		return deny(403, "identity source required", nil)
	}
	external := rule.Identity.Source == "caller_context"
	if !external && (id.Mode != "oauth" || id.Subject == "") {
		return deny(403, "user authentication required", nil)
	}
	if !external && rule.StepUp != nil && !rule.StepUp.Satisfied(id, time.Now()) {
		if owner.Mode != "oauth" {
			return deny(403, "user authentication required", nil)
		}
		w.Header().Set("WWW-Authenticate", rule.StepUp.Challenge())
		return deny(401, "insufficient_user_authentication", rule.StepUp)
	}
	if len(rule.ApprovalServices) == 0 && !external {
		return true
	}
	if external && owner.Mode != "agent-key" && owner.Mode != "oauth" {
		return deny(403, "application authentication required", nil)
	}
	var metadata map[string]json.RawMessage
	_ = json.Unmarshal(meta, &metadata)
	var operation string
	_ = json.Unmarshal(metadata[InvocationOperationKey], &operation)
	if !audit.ValidID(operation) {
		return deny(200, "operation ID required", map[string]any{"metadataKey": InvocationOperationKey, "notExecuted": true})
	}
	digest, details, err := invocation.Content(raw, rule.ShareFields)
	if err != nil {
		return deny(200, "invalid invocation content", nil)
	}
	if len(executionContext) > 0 {
		bound, _ := json.Marshal([]any{digest, executionContext})
		sum := sha256.Sum256(bound)
		digest = hex.EncodeToString(sum[:])
	}
	req := invocation.Request{IdentitySource: rule.Identity.Source, Requirements: invocation.Requirements{StepUp: rule.StepUp, Approval: len(rule.ApprovalServices) > 0}, Version: "1", Phase: "check", OperationID: operation, Identity: id, Tenant: snap.AuditTenant, Upstream: upstream, Tool: tool, Method: msg.Method, Server: vs.Slug, ConfigurationRevision: snap.Revision, PolicyRevision: snap.InvocationSecurity.Revision, Digest: digest, Details: details}
	services := append([]string{}, rule.ApprovalServices...)
	if external {
		up, ok := snap.Upstreams[upstream]
		if !ok || up.CallerContext == nil || !up.CallerContext.Enabled {
			return deny(403, "caller context not configured", nil)
		}
		selected, _, e := up.CallerContext.Prepare(meta, up.Kind)
		if e != nil {
			return deny(403, "invalid caller context", nil)
		}
		var namespaces map[string]map[string]string
		if json.Unmarshal(selected, &namespaces) != nil {
			return deny(403, "caller context required", nil)
		}
		req.CallerContext = map[string]string{}
		for _, field := range rule.Identity.Fields {
			value := namespaces[up.CallerContext.MetaKey][field]
			if value == "" {
				return deny(403, "required identity context missing", nil)
			}
			req.CallerContext[field] = value
		}
		services = []string{rule.Identity.Service}
		for _, name := range rule.ApprovalServices {
			if name != rule.Identity.Service {
				services = append(services, name)
			}
		}
	} else {
		user := id
		req.User = &user
	}
	for _, name := range services {

		svc, ok := snap.InvocationSecurity.Service(upstream, name)
		if !ok {
			return deny(503, "approval service unavailable", nil)
		}
		req.Requirements.Approval = slices.Contains(rule.ApprovalServices, name)
		serviceRequest := req
		if external && name != rule.Identity.Service {
			serviceRequest.CallerContext = nil
		}
		d, e := invocation.Call(ctx, svc, serviceRequest)
		if e != nil {
			return deny(503, "approval service unavailable", nil)
		}
		if d.Status != "allowed" {
			return deny(200, d.Status, map[string]any{"service": name, "decision": d, "operationId": operation, "notExecuted": true})
		}
		if external && name == rule.Identity.Service {
			if d.VerifiedUser == nil || d.VerifiedUser.Subject == "" || d.VerifiedUser.Issuer == "" {
				return deny(403, "identity service did not verify a user", nil)
			}
			req.User = d.VerifiedUser
			if rule.StepUp != nil && !externalAssurance(*rule.StepUp, *req.User) {
				return deny(403, "identity service assurance insufficient", nil)
			}
		}
	}
	if checkAuthorization(ctx) != nil {
		return false
	}
	// Reservation is permanent: ambiguous dispatches must be reconciled, never
	// silently re-executed. A new operation requires a new human approval.
	binding, _ := json.Marshal(req)
	sum := sha256.Sum256(binding)
	if h.ReserveInvocation == nil {
		return deny(503, "invocation replay protection unavailable", nil)
	}
	if err := h.ReserveInvocation(ctx, operation, hex.EncodeToString(sum[:])); err != nil {
		return deny(409, "operation already dispatched or unavailable", map[string]any{"operationId": operation})
	}
	req.Phase = "consume"
	for _, name := range services {
		req.Requirements.Approval = slices.Contains(rule.ApprovalServices, name)
		serviceRequest := req
		if external && name != rule.Identity.Service {
			serviceRequest.CallerContext = nil
		}
		svc, _ := snap.InvocationSecurity.Service(upstream, name) // All services were resolved during check.
		d, e := invocation.Call(ctx, svc, serviceRequest)
		if e != nil || d.Status != "allowed" {
			return deny(403, "approval consumption failed", nil)
		}
		if external && name == rule.Identity.Service && (d.VerifiedUser == nil || d.VerifiedUser.Subject != req.User.Subject || d.VerifiedUser.Issuer != req.User.Issuer || (rule.StepUp != nil && !externalAssurance(*rule.StepUp, *d.VerifiedUser))) {
			return deny(403, "verified user changed before execution", nil)
		}
		h.record(audit.Event{Access: snap.ConsumerAudit(vs), Time: time.Now().UTC(), Revision: snap.Revision, VirtualServer: vs.Slug, Method: msg.Method, Tool: tool, ActorType: owner.Mode, ActorID: owner.CredentialID, Issuer: req.User.Issuer, Subject: req.User.Subject, Status: "ok", Reason: fmt.Sprintf("invocation approval service=%s approval=%s operation=%s policy=%d", name, d.ApprovalID, operation, snap.InvocationSecurity.Revision)})
	}
	return true
}

// Keep assurance freshness live while connecting to an upstream. A cached JWT
// signature or a long-running initialization must not extend the MFA window.
func invocationGuard(ctx context.Context, w http.ResponseWriter, msg *mcp.Message, rule invocation.Rule) context.Context {
	if rule.StepUp == nil || rule.Identity == nil || rule.Identity.Source != "oauth_subject" {
		return ctx
	}
	previous, _ := ctx.Value(authorizationGuardKey{}).(func(context.Context) error)
	id, _ := ctx.Value(assuranceKey{}).(invocation.Identity)
	return context.WithValue(ctx, authorizationGuardKey{}, func(c context.Context) error {
		if previous != nil {
			if err := previous(c); err != nil {
				return err
			}
		}
		if !rule.StepUp.Satisfied(id, time.Now()) {
			w.Header().Set("WWW-Authenticate", rule.StepUp.Challenge())
			response := mcp.NewErrorResponse(msg.ID, -32004, "insufficient_user_authentication")
			response.Error.Data, _ = json.Marshal(rule.StepUp)
			writeMessage(w, 401, response)
			return errAuthorizationChanged
		}
		return nil
	})
}

func externalAssurance(step invocation.StepUp, user invocation.Identity) bool {
	user.Mode = "oauth" // trusted service attestation, not a gateway OAuth credential
	return step.Satisfied(user, time.Now())
}

// Check OAuth assurance before committing an HTTP response to SSE. Execution
// still repeats the check against the policy and enforces approval in callTool.
func (h *Handler) preflightInvocation(ctx context.Context, w http.ResponseWriter, msg *mcp.Message, vs config.VirtualServer, snap *config.Snapshot) bool {
	var p map[string]json.RawMessage
	if json.Unmarshal(msg.Params, &p) != nil {
		return true
	}
	var upstream, tool string
	if msg.Method == mcp.MethodToolsCall {
		var name string
		_ = json.Unmarshal(p["name"], &name)
		vt, ok := vs.Lookup(name)
		if !ok {
			return true
		}
		upstream, tool = vt.UpstreamID, vt.UpstreamName
	} else {
		kind, selector := "resource", "uri"
		if msg.Method == "prompts/get" {
			kind, selector = "prompt", "name"
		}
		completion := msg.Method == "completion/complete"
		if completion {
			var ref map[string]json.RawMessage
			if json.Unmarshal(p["ref"], &ref) != nil {
				return true
			}
			var typ string
			_ = json.Unmarshal(ref["type"], &typ)
			switch typ {
			case "ref/prompt":
				kind, selector = "prompt", "name"
			case "ref/resource":
				kind, selector = "template", "uri"
			default:
				return true
			}
			p = ref
		}
		var exposed string
		_ = json.Unmarshal(p[selector], &exposed)
		object, _, ok := catalogRoute(vs, kind, exposed, !completion)
		if !ok {
			return true
		}
		upstream = object.UpstreamID
	}
	rule, err := snap.InvocationSecurity.Effective(upstream, tool)
	if err != nil || rule.Identity == nil || rule.Identity.Source != "oauth_subject" || (rule.StepUp == nil && len(rule.ApprovalServices) == 0) {
		return true
	}
	id, _ := ctx.Value(assuranceKey{}).(invocation.Identity)
	recordDenied := func(reason string) {
		h.record(audit.Event{Access: snap.ConsumerAudit(vs), Time: time.Now().UTC(), Revision: snap.Revision, VirtualServer: vs.Slug, Method: msg.Method, Tool: tool, ActorType: id.Mode, ActorID: id.CredentialID, Issuer: id.Issuer, Subject: id.Subject, Status: "denied", Reason: reason})
	}
	if id.Mode != "oauth" || id.Subject == "" {
		recordDenied("user authentication required")
		writeMessage(w, 403, mcp.NewErrorResponse(msg.ID, -32004, "user authentication required"))
		return false
	}
	if rule.StepUp != nil && !rule.StepUp.Satisfied(id, time.Now()) {
		recordDenied("insufficient_user_authentication")
		w.Header().Set("WWW-Authenticate", rule.StepUp.Challenge())
		response := mcp.NewErrorResponse(msg.ID, -32004, "insufficient_user_authentication")
		response.Error.Data, _ = json.Marshal(rule.StepUp)
		writeMessage(w, 401, response)
		return false
	}
	return true
}
