package store

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/bishoku/vmcp/core/internal/authz"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// LoadAuthorization reads current policy in one coherent snapshot; no bypass modes exist.
func LoadAuthorization(ctx context.Context, pool *pgxpool.Pool) (authz.Policy, error) {
	if pool == nil {
		return authz.Policy{}, fmt.Errorf("authorization database unavailable")
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return authz.Policy{}, err
	}
	defer tx.Rollback(ctx)
	p, err := readAuthorization(ctx, tx)
	if err != nil {
		return authz.Policy{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return authz.Policy{}, err
	}
	return p, nil
}
func readAuthorization(ctx context.Context, q dbtx) (authz.Policy, error) {
	var p authz.Policy
	if err := q.QueryRow(ctx, `SELECT vmcp_tenant_id(),revision FROM authorization_state WHERE singleton`).Scan(&p.Tenant, &p.Revision); err != nil {
		return p, err
	}
	rows, err := q.Query(ctx, `SELECT r.id::text,r.slug,r.name,r.description,r.builtin,r.version,COALESCE(array_agg(rp.permission_id ORDER BY rp.permission_id) FILTER(WHERE rp.permission_id IS NOT NULL),'{}'::text[]) FROM authorization_roles r LEFT JOIN authorization_role_permissions rp ON rp.role_id=r.id GROUP BY r.id ORDER BY r.id`)
	if err != nil {
		return authz.Policy{}, err
	}
	for rows.Next() {
		var r authz.Role
		if err = rows.Scan(&r.ID, &r.Slug, &r.Name, &r.Description, &r.Builtin, &r.Version, &r.Permissions); err != nil {
			rows.Close()
			return authz.Policy{}, err
		}
		p.Roles = append(p.Roles, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return authz.Policy{}, err
	}
	rows, err = q.Query(ctx, `SELECT id::text,issuer,principal_kind,conditions,enabled,version FROM authorization_mappings ORDER BY id`)
	if err != nil {
		return authz.Policy{}, err
	}
	for rows.Next() {
		var m authz.Mapping
		var raw []byte
		if err = rows.Scan(&m.ID, &m.Issuer, &m.PrincipalKind, &raw, &m.Enabled, &m.Version); err != nil {
			rows.Close()
			return authz.Policy{}, err
		}
		if err = json.Unmarshal(raw, &m.Conditions); err != nil {
			rows.Close()
			return authz.Policy{}, err
		}
		p.Mappings = append(p.Mappings, m)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return authz.Policy{}, err
	}
	rows, err = q.Query(ctx, `SELECT id::text,role_id::text,COALESCE(principal_kind,''),issuer,COALESCE(principal_id,''),COALESCE(mapping_id::text,''),scope_kind,COALESCE(scope_id::text,''),COALESCE(epoch_id::text,''),COALESCE(owner_team_id::text,''),capture_scopes,enabled,version FROM authorization_assignments ORDER BY id`)
	if err != nil {
		return authz.Policy{}, err
	}
	for rows.Next() {
		var a authz.Assignment
		if err = rows.Scan(&a.ID, &a.RoleID, &a.Principal.Kind, &a.Principal.Issuer, &a.Principal.ID, &a.MappingID, &a.Scope.Kind, &a.Scope.ID, &a.Scope.Epoch, &a.Scope.OwnerTeam, &a.CaptureScopes, &a.Enabled, &a.Version); err != nil {
			rows.Close()
			return authz.Policy{}, err
		}
		p.Assignments = append(p.Assignments, a)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return authz.Policy{}, err
	}
	if err = p.Validate(); err != nil {
		return authz.Policy{}, err
	}
	return p, nil
}
