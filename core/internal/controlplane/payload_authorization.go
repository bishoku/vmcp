package controlplane

import (
	"context"
	"net/http"

	"github.com/bishoku/vmcp/core/internal/apierror"
	"github.com/bishoku/vmcp/core/internal/authz"
	"github.com/bishoku/vmcp/core/internal/store"
)

type payloadPolicyKey struct{}
type payloadPolicy struct {
	Policy authz.Policy
}

func payloadRoute(r *http.Request) bool {
	return r.Pattern == "GET /api/v1/audit-events/{event}/payload"
}

// Audit metadata and payloads always use current granular policy.
func (s *Server) preparePayloadPolicy(r *http.Request) (*http.Request, error) {
	if s.Store == nil {
		return r, apierror.New(503, "authorization_unavailable", "Authorization unavailable", "Authorization database unavailable")
	}
	p, err := store.LoadAuthorization(r.Context(), s.Store.Pool)
	if err != nil {
		return r, apierror.New(503, "authorization_unavailable", "Authorization unavailable", "Authorization policy could not be verified")
	}
	return r.WithContext(context.WithValue(r.Context(), payloadPolicyKey{}, payloadPolicy{Policy: p})), nil
}
func payloadPrincipal(p *principal) authz.Principal {
	if p.Via == "oidc" {
		return authz.Principal{Kind: "oidc", Issuer: p.Issuer, ID: p.Subject}
	}
	return authz.Principal{Kind: "control_token", ID: p.ID}
}
func (v payloadPolicy) effective(p *principal) authz.Policy {
	if p == nil {
		return authz.Policy{}
	}
	policy := v.Policy
	who := payloadPrincipal(p)
	if p.Via == "oidc" && p.ClaimsFresh {
		policy = policy.ResolveMappings(who, p.Claims)
	}
	// Platform administrators are explicitly assigned all permissions. Custom role slugs never confer this status.
	if p.Role == "admin" {
		perms := authz.Catalog()
		role := authz.Role{ID: "platform-administrator"}
		for _, spec := range perms {
			role.Permissions = append(role.Permissions, spec.ID)
		}
		policy.Roles = append(append([]authz.Role(nil), policy.Roles...), role)
		policy.Assignments = append(append([]authz.Assignment(nil), policy.Assignments...), authz.Assignment{ID: "platform-administrator", RoleID: role.ID, Principal: who, Scope: authz.Scope{Kind: "tenant"}, Enabled: true, CaptureScopes: []string{"virtual", "upstream", "control", "reads", "system", "omni", "omni_diagnostics", "omni_embedding", "omni_rerank"}})
	}
	return policy
}

func (v payloadPolicy) canRead(p *principal, e authz.AuditPayload) bool {
	if p == nil {
		return false
	}
	return v.effective(p).CanReadPayload(payloadPrincipal(p), e)
}
func auditListRoute(r *http.Request) bool { return r.Pattern == "GET /api/v1/audit-events" }
