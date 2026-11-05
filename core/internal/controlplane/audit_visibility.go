package controlplane

import (
	"encoding/json"
	"net/http"
	"slices"

	"github.com/bishoku/vmcp/core/internal/apierror"
	"github.com/bishoku/vmcp/core/internal/authz"
	"github.com/bishoku/vmcp/core/internal/store"
)

type auditCapabilities struct {
	AuthorizationRevision  int64 `json:"authorizationRevision"`
	CanReadMetadata        bool  `json:"canReadMetadata"`
	CanReadIdentities      bool  `json:"canReadIdentities"`
	CanReadDetails         bool  `json:"canReadDetails"`
	CanReadPayload         bool  `json:"canReadPayload"`
	CanReadRequestPayload  bool  `json:"canReadRequestPayload"`
	CanReadResponsePayload bool  `json:"canReadResponsePayload"`
}

func hasAuditPermission(policy authz.Policy, who authz.Principal, permission string) bool {
	for _, a := range policy.Assignments {
		if !a.Enabled || a.MappingID != "" || a.Principal != who {
			continue
		}
		for _, role := range policy.Roles {
			if role.ID == a.RoleID && slices.Contains(role.Permissions, permission) {
				return true
			}
		}
	}
	return false
}
func (s *Server) auditVisibility(r *http.Request) (*http.Request, payloadPolicy, *principal, bool, error) {
	v, ok := r.Context().Value(payloadPolicyKey{}).(payloadPolicy)
	p, _ := r.Context().Value(principalKey{}).(*principal)
	if !ok || p == nil {
		return r, v, p, false, apierror.New(503, "authorization_unavailable", "Authorization unavailable", "Audit policy unavailable")
	}
	policy := v.effective(p)
	who := payloadPrincipal(p)
	if !hasAuditPermission(policy, who, "audit.events.read") {
		return r, v, p, false, apierror.Forbidden("audit metadata permission required")
	}
	identityFilters := hasAuditPermission(policy, who, "audit.identities.read")
	q := r.URL.Query()
	if !identityFilters && (q.Get("actorId") != "" || q.Get("agentKey") != "" || q.Get("subject") != "") {
		return r, v, p, false, apierror.Forbidden("audit identity filters require identity permission")
	}
	r = r.WithContext(store.WithAuditPolicy(r.Context(), policy, who))
	return r, v, p, identityFilters, nil
}
func (s *Server) auditWireEvents(r *http.Request, events []store.AuditEvent, v payloadPolicy, p *principal) ([]map[string]any, error) {
	ids := make([]string, len(events))
	for i, e := range events {
		ids[i] = e.EventID
	}
	access, err := s.Store.AuditAccessBatch(r.Context(), ids)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(events))
	policy := v.effective(p)
	who := payloadPrincipal(p)
	for _, e := range events {
		context, ok := access[e.EventID]
		if !ok {
			continue
		}
		caps := auditCapabilities{AuthorizationRevision: v.Policy.Revision}
		caps.CanReadMetadata = policy.CanReadAuditPermission(who, context, "audit.events.read")
		if !caps.CanReadMetadata {
			continue
		}
		caps.CanReadIdentities = policy.CanReadAuditPermission(who, context, "audit.identities.read")
		caps.CanReadDetails = policy.CanReadAuditPermission(who, context, "audit.details.read")
		caps.CanReadPayload = policy.CanReadPayload(who, context)
		caps.CanReadRequestPayload = caps.CanReadPayload && e.Direction == "request"
		caps.CanReadResponsePayload = caps.CanReadPayload && e.Direction == "response"
		raw, err := json.Marshal(e)
		if err != nil {
			return nil, err
		}
		var wire map[string]any
		if err = json.Unmarshal(raw, &wire); err != nil {
			return nil, err
		}
		if !caps.CanReadIdentities {
			for _, key := range []string{"actorId", "issuer", "subject", "clientIp", "agentKey"} {
				delete(wire, key)
			}
		}
		if !caps.CanReadDetails {
			delete(wire, "reason")
			delete(wire, "target")
		}
		wire["capabilities"] = caps
		out = append(out, wire)
	}
	return out, nil
}
