package store

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/bishoku/vmcp/core/internal/apierror"
	"github.com/bishoku/vmcp/core/internal/audit"
	"github.com/bishoku/vmcp/core/internal/authz"
	"github.com/jackc/pgx/v5"
)

// AssignmentInput uses immutable IDs. Principal and mapping are mutually exclusive.
// Epoch and project owner pins are persisted, never recomputed during audit reads.
type AssignmentInput struct {
	RoleID        string          `json:"roleId"`
	Principal     authz.Principal `json:"principal"`
	MappingID     string          `json:"mappingId,omitempty"`
	Scope         authz.Scope     `json:"scope"`
	CaptureScopes []string        `json:"captureScopes"`
	Enabled       bool            `json:"enabled"`
}

const assignmentSelect = `SELECT id::text,role_id::text,COALESCE(principal_kind,''),issuer,COALESCE(principal_id,''),COALESCE(mapping_id::text,''),scope_kind,COALESCE(scope_id::text,''),COALESCE(epoch_id::text,''),COALESCE(owner_team_id::text,''),capture_scopes,enabled,version FROM authorization_assignments`

func scanAssignment(row scanner) (authz.Assignment, error) {
	var a authz.Assignment
	err := row.Scan(&a.ID, &a.RoleID, &a.Principal.Kind, &a.Principal.Issuer, &a.Principal.ID, &a.MappingID, &a.Scope.Kind, &a.Scope.ID, &a.Scope.Epoch, &a.Scope.OwnerTeam, &a.CaptureScopes, &a.Enabled, &a.Version)
	return a, err
}
func (t *Tx) AuthorizationAssignment(ctx context.Context, id string) (authz.Assignment, error) {
	if err := authorizationAdmin(ctx); err != nil {
		return authz.Assignment{}, err
	}
	if !audit.ValidID(id) {
		return authz.Assignment{}, roleValidation("/id", "invalid assignment UUID")
	}
	a, err := scanAssignment(t.q.QueryRow(ctx, assignmentSelect+` WHERE id=$1::uuid`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		err = apierror.NotFound("assignment", id)
	}
	return a, err
}
func (t *Tx) assignmentPrincipal(ctx context.Context, in AssignmentInput, trustedIssuer string) error {
	p := in.Principal
	if in.MappingID != "" {
		if !audit.ValidID(in.MappingID) || p != (authz.Principal{}) {
			return roleValidation("/mappingId", "use exactly one principal or mapping")
		}
		var issuer string
		err := t.q.QueryRow(ctx, `SELECT issuer FROM authorization_mappings WHERE id=$1::uuid`, in.MappingID).Scan(&issuer)
		if errors.Is(err, pgx.ErrNoRows) {
			return roleValidation("/mappingId", "mapping does not exist")
		}
		if err != nil {
			return err
		}
		if in.Enabled && (trustedIssuer == "" || issuer != trustedIssuer) {
			return roleValidation("/mappingId", "enabled assignments require the configured OIDC issuer")
		}
		return nil
	}
	switch p.Kind {
	case "control_token":
		if p.Issuer != "" || !audit.ValidID(p.ID) {
			return roleValidation("/principal", "control token requires an immutable token UUID and no issuer")
		}
		var exists, active bool
		if err := t.q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM admin_tokens WHERE id=$1::uuid),EXISTS(SELECT 1 FROM admin_tokens WHERE id=$1::uuid AND revoked_at IS NULL AND (expires_at IS NULL OR expires_at>now()))`, p.ID).Scan(&exists, &active); err != nil {
			return err
		}
		if !exists || in.Enabled && !active {
			return roleValidation("/principal", "token must exist and be active for an enabled assignment")
		}
	case "oidc":
		if strings.TrimSpace(p.ID) == "" || len(p.ID) > 2048 || p.Issuer == "" || len(p.Issuer) > 2048 {
			return roleValidation("/principal", "OIDC issuer and subject are required")
		}
		if in.Enabled && (trustedIssuer == "" || p.Issuer != trustedIssuer) {
			return roleValidation("/principal/issuer", "enabled assignments require the configured OIDC issuer")
		}
	default:
		return roleValidation("/principal/kind", "use oidc or control_token")
	}
	return nil
}
func (t *Tx) assignmentScope(ctx context.Context, s authz.Scope, old *authz.Assignment, auditRole bool) (authz.Scope, error) {
	invalid := func(msg string) (authz.Scope, error) { return s, roleValidation("/scope", msg) }
	for _, id := range []string{s.ID, s.Epoch, s.OwnerTeam} {
		if id != "" && !audit.ValidID(id) {
			return invalid("scope identifiers must be UUIDs")
		}
	}
	switch s.Kind {
	case "tenant", "unmanaged":
		if s.ID != "" || s.Epoch != "" || s.OwnerTeam != "" {
			return invalid("tenant and unmanaged scopes cannot contain IDs or pins")
		}
		return s, nil
	case "team":
		if s.ID == "" || s.Epoch != "" || s.OwnerTeam != "" {
			return invalid("team scope requires only a team UUID")
		}
		var exists bool
		if err := t.q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM teams WHERE id=$1::uuid)`, s.ID).Scan(&exists); err != nil {
			return s, err
		}
		if !exists {
			return invalid("team does not exist")
		}
		return s, nil
	case "project":
		if s.ID == "" || s.Epoch != "" {
			return invalid("project requires a project UUID and no resource epoch")
		}
		var owner string
		err := t.q.QueryRow(ctx, `SELECT COALESCE(owner_team_id::text,'') FROM projects WHERE id=$1::uuid`, s.ID).Scan(&owner)
		if errors.Is(err, pgx.ErrNoRows) {
			return invalid("project does not exist")
		}
		if err != nil {
			return s, err
		}
		if old != nil && old.Scope.Kind == s.Kind && old.Scope.ID == s.ID && s.OwnerTeam == "" {
			s.OwnerTeam = old.Scope.OwnerTeam
			return s, nil
		}
		if s.OwnerTeam != "" && s.OwnerTeam != owner {
			var historical bool
			if err := t.q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM audit_resource_epochs WHERE project_id=$1::uuid AND owner_team_id=$2::uuid AND tenant_id=vmcp_tenant_id())`, s.ID, s.OwnerTeam).Scan(&historical); err != nil {
				return s, err
			}
			if !historical {
				return invalid("owner does not match current or recorded project ownership")
			}
		} else {
			s.OwnerTeam = owner
		}
		return s, nil
	case "upstream", "virtual_server":
		if s.ID == "" {
			return invalid("resource UUID required")
		}
		if old != nil && old.Scope.Kind == s.Kind && old.Scope.ID == s.ID && s.Epoch == "" {
			s.Epoch = old.Scope.Epoch
		}
		if s.Epoch != "" {
			var owner string
			err := t.q.QueryRow(ctx, `SELECT COALESCE(owner_team_id::text,'') FROM audit_resource_epochs WHERE epoch_id=$1::uuid AND resource_kind=$2 AND resource_id=$3::uuid AND tenant_id=vmcp_tenant_id()`, s.Epoch, s.Kind, s.ID).Scan(&owner)
			if errors.Is(err, pgx.ErrNoRows) {
				return invalid("epoch does not belong to this resource")
			}
			if err != nil {
				return s, err
			}
			if s.OwnerTeam != "" && s.OwnerTeam != owner {
				return invalid("owner does not match the recorded epoch")
			}
			s.OwnerTeam = owner
			return s, nil
		}
		var epoch, owner string
		err := t.q.QueryRow(ctx, `SELECT epoch_id::text,COALESCE(owner_team_id::text,'') FROM audit_resource_epochs WHERE resource_kind=$1 AND resource_id=$2::uuid AND end_revision IS NULL AND tenant_id=vmcp_tenant_id()`, s.Kind, s.ID).Scan(&epoch, &owner)
		if errors.Is(err, pgx.ErrNoRows) {
			return invalid("live resource does not exist")
		}
		if err != nil {
			return s, err
		}
		if s.OwnerTeam != "" && s.OwnerTeam != owner {
			return invalid("owner does not match the resource")
		}
		s.OwnerTeam = owner
		if auditRole {
			s.Epoch = epoch
		}
		return s, nil
	default:
		return invalid("unknown scope kind")
	}
}

func (t *Tx) SaveAuthorizationAssignment(ctx context.Context, id string, in AssignmentInput, expected *int64, trustedIssuer string) (authz.Assignment, error) {
	var out authz.Assignment
	if err := t.lockAuthorization(ctx); err != nil {
		return out, err
	}
	var old *authz.Assignment
	if id != "" {
		a, err := t.AuthorizationAssignment(ctx, id)
		if err != nil {
			return out, err
		}
		if expected == nil || a.Version != *expected {
			return out, apierror.PreconditionFailed()
		}
		old = &a
	}
	if !audit.ValidID(in.RoleID) {
		return out, roleValidation("/roleId", "role UUID required")
	}
	var permissions []string
	var slug string
	err := t.q.QueryRow(ctx, `SELECT r.slug,COALESCE(array_agg(p.permission_id) FILTER(WHERE p.permission_id IS NOT NULL),'{}'::text[]) FROM authorization_roles r LEFT JOIN authorization_role_permissions p ON p.role_id=r.id WHERE r.id=$1::uuid GROUP BY r.id`, in.RoleID).Scan(&slug, &permissions)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, roleValidation("/roleId", "role does not exist")
	}
	if err != nil {
		return out, err
	}
	if err = t.assignmentPrincipal(ctx, in, trustedIssuer); err != nil {
		return out, err
	}
	seen := map[string]bool{}
	for _, scope := range in.CaptureScopes {
		if !authz.ValidCaptureScope(scope) || seen[scope] {
			return out, roleValidation("/captureScopes", "capture scopes must be known and unique")
		}
		seen[scope] = true
	}
	auditRole, capture, applicable := false, false, len(permissions) == 0
	for _, id := range permissions {
		p, _ := authz.Lookup(id)
		auditRole = auditRole || strings.HasPrefix(id, "audit.")
		capture = capture || p.Capture
		applicable = applicable || slices.Contains(p.Scopes, in.Scope.Kind)
	}
	if !applicable {
		return out, roleValidation("/scope", "none of the role permissions support this scope")
	}
	if capture && len(in.CaptureScopes) == 0 {
		return out, roleValidation("/captureScopes", "payload permissions require explicit capture scopes")
	}
	in.Scope, err = t.assignmentScope(ctx, in.Scope, old, auditRole)
	if err != nil {
		return out, err
	}
	if in.CaptureScopes == nil {
		in.CaptureScopes = []string{}
	}
	args := []any{in.RoleID, in.Principal.Kind, in.Principal.Issuer, in.Principal.ID, in.MappingID, in.Scope.Kind, in.Scope.ID, in.Scope.Epoch, in.Scope.OwnerTeam, in.CaptureScopes, in.Enabled}
	if id == "" {
		err = t.q.QueryRow(ctx, `INSERT INTO authorization_assignments(role_id,principal_kind,issuer,principal_id,mapping_id,scope_kind,scope_id,epoch_id,owner_team_id,capture_scopes,enabled) VALUES($1::uuid,NULLIF($2,''),$3,NULLIF($4,''),NULLIF($5,'')::uuid,$6,NULLIF($7,'')::uuid,NULLIF($8,'')::uuid,NULLIF($9,'')::uuid,$10,$11) RETURNING id::text`, args...).Scan(&id)
	} else {
		args = append(args, id)
		_, err = t.q.Exec(ctx, `UPDATE authorization_assignments SET role_id=$1::uuid,principal_kind=NULLIF($2,''),issuer=$3,principal_id=NULLIF($4,''),mapping_id=NULLIF($5,'')::uuid,scope_kind=$6,scope_id=NULLIF($7,'')::uuid,epoch_id=NULLIF($8,'')::uuid,owner_team_id=NULLIF($9,'')::uuid,capture_scopes=$10,enabled=$11,version=version+1 WHERE id=$12::uuid`, args...)
	}
	if err != nil {
		return out, err
	}
	t.auditTarget("role-assignment", id)
	return t.AuthorizationAssignment(ctx, id)
}
func (t *Tx) DeleteAuthorizationAssignment(ctx context.Context, id string, expected int64) error {
	if err := t.lockAuthorization(ctx); err != nil {
		return err
	}
	a, err := t.AuthorizationAssignment(ctx, id)
	if err != nil {
		return err
	}
	if a.Version != expected {
		return apierror.PreconditionFailed()
	}
	if _, err = t.q.Exec(ctx, `DELETE FROM authorization_assignments WHERE id=$1::uuid`, id); err != nil {
		return err
	}
	t.auditTarget("role-assignment", id)
	return nil
}
