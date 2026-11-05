package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"unicode"

	"github.com/bishoku/vmcp/core/internal/apierror"
	"github.com/bishoku/vmcp/core/internal/audit"
	"github.com/bishoku/vmcp/core/internal/authz"
	"github.com/jackc/pgx/v5"
)

// A mapping can select assignments, but cannot establish an issuer trust root.
// Disabled drafts are allowed before configuring SSO; enabling always rechecks it.
type MappingInput struct {
	Issuer        string            `json:"issuer"`
	PrincipalKind string            `json:"principalKind"`
	Conditions    []authz.Condition `json:"conditions"`
	Enabled       bool              `json:"enabled"`
}

func validateMapping(in MappingInput, trustedIssuer string) error {
	if strings.TrimSpace(in.Issuer) == "" || len(in.Issuer) > 2048 || in.PrincipalKind != "oidc" {
		return roleValidation("/issuer", "OIDC kind and a nonempty issuer (maximum 2048 bytes) are required")
	}
	if in.Enabled && (trustedIssuer == "" || in.Issuer != trustedIssuer) {
		return roleValidation("/issuer", "enabled mappings require the configured OIDC issuer")
	}
	if len(in.Conditions) < 1 || len(in.Conditions) > 32 {
		return roleValidation("/conditions", "one to 32 AND conditions are required")
	}
	seen := map[authz.Condition]bool{}
	for _, c := range in.Conditions {
		if strings.TrimSpace(c.Path) == "" || len(c.Path) > 512 || strings.TrimSpace(c.Value) == "" || len(c.Value) > 2048 || seen[c] {
			return roleValidation("/conditions", "condition paths and values must be nonempty, bounded and unique")
		}
		seen[c] = true
		switch c.Operator {
		case "equals", "array_contains":
		case "scope_contains":
			if strings.IndexFunc(c.Value, unicode.IsSpace) >= 0 {
				return roleValidation("/conditions", "scope_contains requires a single exact scope token")
			}
		default:
			return roleValidation("/conditions", "use equals, array_contains or scope_contains")
		}
	}
	return nil
}
func (t *Tx) AuthorizationMapping(ctx context.Context, id string) (authz.Mapping, error) {
	var m authz.Mapping
	if err := authorizationAdmin(ctx); err != nil {
		return m, err
	}
	if !audit.ValidID(id) {
		return m, roleValidation("/id", "mapping UUID required")
	}
	var raw []byte
	err := t.q.QueryRow(ctx, `SELECT id::text,issuer,principal_kind,conditions,enabled,version FROM authorization_mappings WHERE id=$1::uuid`, id).Scan(&m.ID, &m.Issuer, &m.PrincipalKind, &raw, &m.Enabled, &m.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return m, apierror.NotFound("mapping", id)
	}
	if err != nil {
		return m, err
	}
	err = json.Unmarshal(raw, &m.Conditions)
	return m, err
}
func (t *Tx) SaveAuthorizationMapping(ctx context.Context, id string, in MappingInput, expected *int64, trustedIssuer string) (authz.Mapping, error) {
	var out authz.Mapping
	if err := t.lockAuthorization(ctx); err != nil {
		return out, err
	}
	if id != "" {
		old, err := t.AuthorizationMapping(ctx, id)
		if err != nil {
			return out, err
		}
		if expected == nil || old.Version != *expected {
			return out, apierror.PreconditionFailed()
		}
	}
	if err := validateMapping(in, trustedIssuer); err != nil {
		return out, err
	}
	// A referenced enabled assignment must never be repointed to an untrusted
	// issuer, even by editing a disabled mapping draft underneath the assignment.
	if id != "" && in.Issuer != trustedIssuer {
		var active bool
		if err := t.q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM authorization_assignments WHERE mapping_id=$1::uuid AND enabled)`, id).Scan(&active); err != nil {
			return out, err
		}
		if active {
			return out, apierror.InUse("disable mapping assignments before changing their issuer outside the configured provider")
		}
	}
	raw, err := json.Marshal(in.Conditions)
	if err != nil {
		return out, err
	}
	if id == "" {
		err = t.q.QueryRow(ctx, `INSERT INTO authorization_mappings(issuer,principal_kind,conditions,enabled) VALUES($1,$2,$3,$4) RETURNING id::text`, in.Issuer, in.PrincipalKind, raw, in.Enabled).Scan(&id)
	} else {
		_, err = t.q.Exec(ctx, `UPDATE authorization_mappings SET issuer=$2,principal_kind=$3,conditions=$4,enabled=$5,version=version+1 WHERE id=$1::uuid`, id, in.Issuer, in.PrincipalKind, raw, in.Enabled)
	}
	if err != nil {
		return out, err
	}
	t.auditTarget("identity-mapping", id)
	return t.AuthorizationMapping(ctx, id)
}
func (t *Tx) DeleteAuthorizationMapping(ctx context.Context, id string, expected int64) error {
	if err := t.lockAuthorization(ctx); err != nil {
		return err
	}
	old, err := t.AuthorizationMapping(ctx, id)
	if err != nil {
		return err
	}
	if old.Version != expected {
		return apierror.PreconditionFailed()
	}
	var used bool
	if err := t.q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM authorization_assignments WHERE mapping_id=$1::uuid)`, id).Scan(&used); err != nil {
		return err
	}
	if used {
		return apierror.InUse("remove mapping assignments, including disabled assignments, before deleting this mapping")
	}
	_, err = t.q.Exec(ctx, `DELETE FROM authorization_mappings WHERE id=$1::uuid`, id)
	if err == nil {
		t.auditTarget("identity-mapping", id)
	}
	return err
}
