package store

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/bishoku/vmcp/core/internal/apierror"
	"github.com/bishoku/vmcp/core/internal/authz"
	"github.com/jackc/pgx/v5"
)

// RoleInput deliberately excludes IDs, builtin flags and versions. Copies freeze
// the source permission set; later builtin upgrades do not expand a custom role.
type RoleInput struct {
	Slug        string   `json:"slug"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
	CopyFrom    string   `json:"copyFrom,omitempty"`
}

func authorizationAdmin(ctx context.Context) error {
	a, ok := AccessFrom(ctx)
	if !ok || a.Role != "admin" {
		return apierror.Forbidden("platform admin required for authorization management")
	}
	return nil
}
func roleValidation(field, message string) error {
	return apierror.Validation([]apierror.FieldError{{Field: field, Message: message}})
}
func (t *Tx) AuthorizationRole(ctx context.Context, slug string) (authz.Role, error) {
	var r authz.Role
	if err := authorizationAdmin(ctx); err != nil {
		return r, err
	}
	err := t.q.QueryRow(ctx, `SELECT r.id::text,r.slug,r.name,r.description,r.builtin,r.version,COALESCE(array_agg(p.permission_id ORDER BY p.permission_id) FILTER(WHERE p.permission_id IS NOT NULL),'{}'::text[]) FROM authorization_roles r LEFT JOIN authorization_role_permissions p ON p.role_id=r.id WHERE r.slug=$1 GROUP BY r.id`, slug).Scan(&r.ID, &r.Slug, &r.Name, &r.Description, &r.Builtin, &r.Version, &r.Permissions)
	if errors.Is(err, pgx.ErrNoRows) {
		err = apierror.NotFound("role", slug)
	}
	return r, err
}
func (t *Tx) lockAuthorization(ctx context.Context) error {
	if err := authorizationAdmin(ctx); err != nil {
		return err
	}
	var rev int64
	return t.q.QueryRow(ctx, `SELECT revision FROM authorization_state WHERE singleton FOR UPDATE`).Scan(&rev)
}
func (t *Tx) SaveAuthorizationRole(ctx context.Context, slug string, in RoleInput, expected *int64) (authz.Role, error) {
	var out authz.Role
	if err := t.lockAuthorization(ctx); err != nil {
		return out, err
	}
	creating := slug == ""
	if creating {
		slug = in.Slug
	} else {
		old, err := t.AuthorizationRole(ctx, slug)
		if err != nil {
			return out, err
		}
		if old.Builtin {
			return out, apierror.Forbidden("builtin roles cannot be changed; create a copy")
		}
		if expected == nil || *expected != old.Version {
			return out, apierror.PreconditionFailed()
		}
		if in.Slug != "" && in.Slug != slug {
			return out, roleValidation("/slug", "role slug is immutable")
		}
		if in.CopyFrom != "" {
			return out, roleValidation("/copyFrom", "copyFrom is only available when creating a role")
		}
	}
	if !ValidSlug(slug) {
		return out, roleValidation("/slug", "invalid role slug")
	}
	if strings.TrimSpace(in.Name) == "" || len(in.Name) > 200 || len(in.Description) > 4000 {
		return out, roleValidation("/name", "name is required (maximum 200 bytes); description maximum is 4000 bytes")
	}
	if in.CopyFrom != "" {
		if in.Permissions != nil {
			return out, roleValidation("/permissions", "permissions and copyFrom are mutually exclusive")
		}
		src, err := t.AuthorizationRole(ctx, in.CopyFrom)
		if err != nil {
			return out, err
		}
		in.Permissions = src.Permissions
	}
	if in.Permissions == nil {
		return out, roleValidation("/permissions", "an explicit permission list or copyFrom is required")
	}
	seen := map[string]bool{}
	capture := false
	auditRole := false
	for _, id := range in.Permissions {
		spec, ok := authz.Lookup(id)
		if !ok || seen[id] {
			return out, roleValidation("/permissions", "permissions must be registered and unique")
		}
		seen[id] = true
		capture = capture || spec.Capture
		auditRole = auditRole || strings.HasPrefix(id, "audit.")
	}
	if !creating && capture {
		var incompatible bool
		if err := t.q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM authorization_assignments a JOIN authorization_roles r ON r.id=a.role_id WHERE r.slug=$1 AND cardinality(a.capture_scopes)=0)`, slug).Scan(&incompatible); err != nil {
			return out, err
		}
		if incompatible {
			return out, apierror.InUse("set explicit capture scopes on existing assignments before adding payload permissions")
		}
	}
	if !creating && auditRole {
		var unpinned bool
		if err := t.q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM authorization_assignments a JOIN authorization_roles r ON r.id=a.role_id WHERE r.slug=$1 AND a.scope_kind IN ('upstream','virtual_server') AND a.epoch_id IS NULL)`, slug).Scan(&unpinned); err != nil {
			return out, err
		}
		if unpinned {
			return out, apierror.InUse("pin resource assignment epochs before adding audit permissions")
		}
	}
	if creating {
		err := t.q.QueryRow(ctx, `INSERT INTO authorization_roles(slug,name,description) VALUES($1,$2,$3) RETURNING id::text`, slug, in.Name, in.Description).Scan(&out.ID)
		if err != nil {
			return out, mapWriteErr(err, "role", slug)
		}
	} else {
		if _, err := t.q.Exec(ctx, `UPDATE authorization_roles SET name=$2,description=$3,version=version+1 WHERE slug=$1`, slug, in.Name, in.Description); err != nil {
			return out, err
		}
		if err := t.q.QueryRow(ctx, `SELECT id::text FROM authorization_roles WHERE slug=$1`, slug).Scan(&out.ID); err != nil {
			return out, err
		}
		if _, err := t.q.Exec(ctx, `DELETE FROM authorization_role_permissions WHERE role_id=$1::uuid`, out.ID); err != nil {
			return out, err
		}
	}
	slices.Sort(in.Permissions)
	for _, id := range in.Permissions {
		if _, err := t.q.Exec(ctx, `INSERT INTO authorization_role_permissions(role_id,permission_id) VALUES($1::uuid,$2)`, out.ID, id); err != nil {
			return out, err
		}
	}
	t.auditTarget("role", slug)
	return t.AuthorizationRole(ctx, slug)
}
func (t *Tx) DeleteAuthorizationRole(ctx context.Context, slug string, expected int64) error {
	if err := t.lockAuthorization(ctx); err != nil {
		return err
	}
	r, err := t.AuthorizationRole(ctx, slug)
	if err != nil {
		return err
	}
	if r.Builtin {
		return apierror.Forbidden("builtin roles cannot be deleted")
	}
	if r.Version != expected {
		return apierror.PreconditionFailed()
	}
	var used bool
	if err := t.q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM authorization_assignments WHERE role_id=$1::uuid)`, r.ID).Scan(&used); err != nil {
		return err
	}
	if used {
		return apierror.InUse("remove role assignments before deleting this role")
	}
	_, err = t.q.Exec(ctx, `DELETE FROM authorization_roles WHERE id=$1::uuid`, r.ID)
	if err == nil {
		t.auditTarget("role", slug)
	}
	return err
}
