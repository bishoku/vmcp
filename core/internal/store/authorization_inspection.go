package store

import (
	"context"
	"strings"

	"github.com/bishoku/vmcp/core/internal/apierror"
	"github.com/bishoku/vmcp/core/internal/audit"
	"github.com/bishoku/vmcp/core/internal/authz"
)

type AuthorizationEvaluationInput struct {
	Principal    authz.Principal     `json:"principal"`
	Claims       map[string]any      `json:"claims,omitempty"`
	Permission   string              `json:"permission"`
	Resource     *EvaluationResource `json:"resource,omitempty"`
	AuditEventID string              `json:"auditEventId,omitempty"`
}
type EvaluationResource struct {
	Kind string `json:"kind"`
	ID   string `json:"id,omitempty"`
}
type EvaluationDecision struct {
	Allowed      bool   `json:"allowed"`
	Reason       string `json:"reason"`
	AssignmentID string `json:"assignmentId,omitempty"`
}
type AuthorizationEvaluation struct {
	SimulationOnly          bool               `json:"simulationOnly"`
	AuthenticationValidated bool               `json:"authenticationValidated"`
	AuthorizationRevision   int64              `json:"authorizationRevision"`
	ClaimMappings           string             `json:"claimMappings"`
	MatchedMappingIDs       []string           `json:"matchedMappingIds"`
	CandidateAssignmentIDs  []string           `json:"candidateAssignmentIds"`
	Decision                EvaluationDecision `json:"decision"`
}

// EvaluateAuthorization is an administrative what-if operation. Samples are not
// verified tokens, do not enter a session, and cannot grant the calling user rights.
// No positive result claims authentication, claim freshness or platform administrator status.
func (s *Store) EvaluateAuthorization(ctx context.Context, in AuthorizationEvaluationInput, trustedIssuer string) (AuthorizationEvaluation, error) {
	out := AuthorizationEvaluation{SimulationOnly: true, ClaimMappings: "simulation-only", MatchedMappingIDs: []string{}, CandidateAssignmentIDs: []string{}}
	if err := authorizationAdmin(ctx); err != nil {
		return out, err
	}
	if _, ok := authz.Lookup(in.Permission); !ok {
		return out, roleValidation("/permission", "registered permission required")
	}
	if (in.Resource == nil) == (in.AuditEventID == "") {
		return out, roleValidation("/resource", "select exactly one resource or persisted audit event")
	}
	if in.AuditEventID != "" && !audit.ValidID(in.AuditEventID) {
		return out, roleValidation("/auditEventId", "audit event UUID required")
	}
	if in.Principal.Kind != "oidc" && len(in.Claims) > 0 {
		return out, roleValidation("/claims", "claim simulation is only available for OIDC principals")
	}
	for _, field := range []struct{ key, value string }{{"iss", in.Principal.Issuer}, {"sub", in.Principal.ID}} {
		if value, exists := in.Claims[field.key]; exists {
			str, ok := value.(string)
			if !ok || str != field.value {
				return out, roleValidation("/claims", "sample iss/sub must match the selected principal")
			}
		}
	}
	err := s.View(ctx, func(t *Tx) error {
		if err := t.assignmentPrincipal(ctx, AssignmentInput{Principal: in.Principal, Enabled: true}, trustedIssuer); err != nil {
			return err
		}
		p, err := readAuthorization(ctx, t.q)
		if err != nil {
			return err
		}
		out.AuthorizationRevision = p.Revision

		for _, m := range p.Mappings {
			if m.Match(in.Principal, in.Claims) {
				out.MatchedMappingIDs = append(out.MatchedMappingIDs, m.ID)
			}
		}
		p = p.ResolveMappings(in.Principal, in.Claims)
		for _, a := range p.Assignments {
			if a.Enabled && a.MappingID == "" && a.Principal == in.Principal {
				out.CandidateAssignmentIDs = append(out.CandidateAssignmentIDs, a.ID)
			}
		}
		if in.AuditEventID != "" {
			// Use immutable persisted provenance, never sample-supplied audit domains.
			events, err := auditAccessBatch(ctx, t.q, []string{in.AuditEventID})
			if err != nil {
				return err
			}
			e, ok := events[in.AuditEventID]
			if !ok {
				return apierror.NotFound("audit event", in.AuditEventID)
			}
			allowed := false
			switch in.Permission {
			case "audit.events.read", "audit.identities.read", "audit.details.read":
				allowed = p.CanReadAuditPermission(in.Principal, e, in.Permission)
			case "audit.payloads.request.read", "audit.payloads.response.read":
				direction := strings.TrimSuffix(strings.TrimPrefix(in.Permission, "audit.payloads."), ".read")
				allowed = e.Direction == direction && p.CanReadPayload(in.Principal, e)
			default:
				return roleValidation("/permission", "event simulation supports metadata, identity, details and directional payload read permissions")
			}
			reason := "audit_policy_denied"
			if allowed {
				reason = "audit_policy_allowed"
			}
			out.Decision = EvaluationDecision{Allowed: allowed, Reason: reason}
			return nil
		}
		if strings.HasPrefix(in.Permission, "audit.") && (in.Permission == "audit.events.read" || in.Permission == "audit.identities.read" || in.Permission == "audit.details.read" || strings.HasPrefix(in.Permission, "audit.payloads.")) {
			return roleValidation("/auditEventId", "audit visibility simulation requires a persisted event")
		}
		r, err := t.evaluationResource(ctx, p.Tenant, *in.Resource)
		if err != nil {
			return err
		}
		d := p.Decide(authz.Request{Principal: in.Principal, Permission: in.Permission, Resource: r})
		out.Decision = EvaluationDecision{Allowed: d.Allowed, Reason: d.Reason, AssignmentID: d.AssignmentID}
		return nil
	})
	return out, err
}
func (t *Tx) evaluationResource(ctx context.Context, tenant string, in EvaluationResource) (authz.Resource, error) {
	r := authz.Resource{Tenant: tenant, Kind: in.Kind, ID: in.ID}
	if in.Kind == "tenant" {
		if in.ID != "" {
			return r, roleValidation("/resource/id", "tenant resource has no ID")
		}
		return r, nil
	}
	if !audit.ValidID(in.ID) {
		return r, roleValidation("/resource/id", "resource UUID required")
	}
	var exists bool
	switch in.Kind {
	case "team":
		if err := t.q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM teams WHERE id=$1::uuid)`, in.ID).Scan(&exists); err != nil {
			return r, err
		}
		r.OwnerTeam = in.ID
	case "project":
		if err := t.q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM projects WHERE id=$1::uuid),COALESCE((SELECT owner_team_id::text FROM projects WHERE id=$1::uuid),'')`, in.ID).Scan(&exists, &r.OwnerTeam); err != nil {
			return r, err
		}
		r.Project = in.ID
	case "upstream", "virtual_server":
		if err := t.q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM audit_resource_epochs WHERE resource_kind=$1 AND resource_id=$2::uuid AND end_revision IS NULL AND tenant_id=vmcp_tenant_id()),COALESCE((SELECT owner_team_id::text FROM audit_resource_epochs WHERE resource_kind=$1 AND resource_id=$2::uuid AND end_revision IS NULL AND tenant_id=vmcp_tenant_id()),''),COALESCE((SELECT project_id::text FROM audit_resource_epochs WHERE resource_kind=$1 AND resource_id=$2::uuid AND end_revision IS NULL AND tenant_id=vmcp_tenant_id()),''),COALESCE((SELECT epoch_id::text FROM audit_resource_epochs WHERE resource_kind=$1 AND resource_id=$2::uuid AND end_revision IS NULL AND tenant_id=vmcp_tenant_id()),'')`, in.Kind, in.ID).Scan(&exists, &r.OwnerTeam, &r.Project, &r.Epoch); err != nil {
			return r, err
		}
	default:
		return r, roleValidation("/resource/kind", "use tenant, team, project, upstream or virtual_server")
	}
	if !exists {
		return r, apierror.NotFound(in.Kind, in.ID)
	}
	return r, nil
}

type AuthorizationStatus struct {
	AuthorizationRevision int64            `json:"authorizationRevision"`
	SchemaVersion         int64            `json:"schemaVersion"`
	TrustedIssuer         string           `json:"trustedIssuer"`
	ClaimMappings         string           `json:"claimMappings"`
	ExternalOAuthBearer   bool             `json:"externalOAuthBearer"`
	Blockers              []string         `json:"blockers"`
	Counts                map[string]int64 `json:"counts"`
}

// Status reports facts rather than activating or certifying a migration. All SQL
// counters and policy come from one repeatable-read snapshot. No backfill occurs.
func (s *Store) AuthorizationStatus(ctx context.Context, trustedIssuer string) (AuthorizationStatus, error) {
	out := AuthorizationStatus{TrustedIssuer: trustedIssuer, ClaimMappings: "simulation-only", Blockers: []string{"oidc_claim_sessions_not_connected"}, Counts: map[string]int64{}}
	if err := authorizationAdmin(ctx); err != nil {
		return out, err
	}
	err := s.View(ctx, func(t *Tx) error {
		if err := t.q.QueryRow(ctx, `SELECT revision FROM authorization_state WHERE singleton`).Scan(&out.AuthorizationRevision); err != nil {
			return err
		}
		if err := t.q.QueryRow(ctx, `SELECT COALESCE(max(version_id),0) FROM goose_db_version WHERE is_applied`).Scan(&out.SchemaVersion); err != nil {
			return err
		}
		// Counts contain neither subjects, claim values, raw bodies nor credentials.
		rows, err := t.q.Query(ctx, `SELECT 'roles',count(*) FROM authorization_roles UNION ALL SELECT 'customRoles',count(*) FROM authorization_roles WHERE NOT builtin UNION ALL SELECT 'assignments',count(*) FROM authorization_assignments UNION ALL SELECT 'enabledAssignments',count(*) FROM authorization_assignments WHERE enabled UNION ALL SELECT 'mappings',count(*) FROM authorization_mappings UNION ALL SELECT 'enabledMappings',count(*) FROM authorization_mappings WHERE enabled UNION ALL SELECT 'untrustedEnabledMappings',count(*) FROM authorization_mappings WHERE enabled AND (issuer<>$1 OR $1='') UNION ALL SELECT 'untrustedEnabledOIDCAssignments',count(*) FROM authorization_assignments a LEFT JOIN authorization_mappings m ON m.id=a.mapping_id WHERE a.enabled AND (a.principal_kind='oidc' AND (a.issuer<>$1 OR $1='') OR a.mapping_id IS NOT NULL AND (m.issuer<>$1 OR $1='')) UNION ALL SELECT 'inactiveTokenAssignments',count(*) FROM authorization_assignments a WHERE a.enabled AND a.principal_kind='control_token' AND NOT EXISTS(SELECT 1 FROM admin_tokens t WHERE t.id::text=a.principal_id AND t.revoked_at IS NULL AND (t.expires_at IS NULL OR t.expires_at>now())) UNION ALL SELECT 'unpinnedAuditResourceAssignments',count(*) FROM authorization_assignments a WHERE a.enabled AND a.scope_kind IN ('upstream','virtual_server') AND a.epoch_id IS NULL AND EXISTS(SELECT 1 FROM authorization_role_permissions rp WHERE rp.role_id=a.role_id AND rp.permission_id LIKE 'audit.%') UNION ALL SELECT 'legacyAuditEvents',count(*) FROM audit_events e LEFT JOIN audit_access_contexts c USING(event_id) WHERE e.tenant_id=vmcp_tenant_id() AND (c.event_id IS NULL OR c.classification='legacy') UNION ALL SELECT 'resourceAuditEvents',count(*) FROM audit_access_contexts WHERE tenant_id=vmcp_tenant_id() AND classification='resource' UNION ALL SELECT 'tenantAuditEvents',count(*) FROM audit_access_contexts WHERE tenant_id=vmcp_tenant_id() AND classification='tenant' UNION ALL SELECT 'legacyBrowserSessions',count(*) FROM browser_sessions WHERE model_version=0 AND revoked_at IS NULL AND expires_at>now() UNION ALL SELECT 'serverBrowserSessions',count(*) FROM browser_sessions WHERE model_version=1 AND revoked_at IS NULL AND expires_at>now() UNION ALL SELECT 'expiredBrowserSessions',count(*) FROM browser_sessions WHERE expires_at<=now()`, trustedIssuer)
		if err != nil {
			return err
		}
		for rows.Next() {
			var k string
			var n int64
			if err := rows.Scan(&k, &n); err != nil {
				rows.Close()
				return err
			}
			out.Counts[k] = n
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for _, k := range []string{"untrustedEnabledMappings", "untrustedEnabledOIDCAssignments", "inactiveTokenAssignments", "unpinnedAuditResourceAssignments"} {
			if out.Counts[k] > 0 {
				out.Blockers = append(out.Blockers, k)
			}
		}
		return nil
	})
	return out, err
}
