package store

import (
	"context"
	"errors"

	"github.com/bishoku/vmcp/core/internal/audit"
	"github.com/jackc/pgx/v5"
)

// ControlAuditContext pins server-owned identity before request admission. Slugs
// locate current resources only here, never when authorizing historical payloads.
// Missing targets conservatively remain tenant scoped (including PUT creates).
func (s *Store) ControlAuditContext(ctx context.Context, kind, slug string) (*audit.AccessContext, error) {
	tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	c := &audit.AccessContext{Classification: "tenant", Origin: "control"}
	if err = tx.QueryRow(ctx, `SELECT vmcp_tenant_id(),revision FROM config_revision`).Scan(&c.Tenant, &c.Revision); err != nil {
		return nil, err
	}
	var query string
	switch kind {
	case "upstream":
		query = `SELECT e.resource_id::text,COALESCE(e.project_id::text,''),COALESCE(e.owner_team_id::text,''),e.epoch_id::text FROM upstreams u JOIN audit_resource_epochs e ON e.resource_kind='upstream' AND e.resource_id=u.id AND e.end_revision IS NULL AND e.tenant_id=vmcp_tenant_id() WHERE u.slug=$1 AND u.tenant_id=` + tenantSQL
	case "virtual_server":
		query = `SELECT e.resource_id::text,COALESCE(e.project_id::text,''),COALESCE(e.owner_team_id::text,''),e.epoch_id::text FROM virtual_servers v JOIN audit_resource_epochs e ON e.resource_kind='virtual_server' AND e.resource_id=v.id AND e.end_revision IS NULL AND e.tenant_id=vmcp_tenant_id() WHERE v.slug=$1 AND v.tenant_id=` + tenantSQL
	}
	if query != "" {
		d := audit.AccessDomain{Kind: "target", ResourceKind: kind}
		err = tx.QueryRow(ctx, query, slug).Scan(&d.ResourceID, &d.ProjectID, &d.OwnerTeamID, &d.EpochID)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
		if err == nil {
			c.Classification = "resource"
			c.Domains = []audit.AccessDomain{d}
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return c, nil
}

// controlAuditCurrent is also used inside Store.Do after taking the configuration
// revision lock, so a scoped write cannot silently follow an ownership transfer.
func controlAuditCurrent(ctx context.Context, q dbtx, c *audit.AccessContext) (bool, error) {
	if c == nil || c.Origin != "control" || c.Classification != "resource" {
		return true, nil
	}
	if len(c.Domains) != 1 {
		return false, nil
	}
	d := c.Domains[0]
	var current bool
	err := q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM audit_resource_epochs WHERE tenant_id=$1 AND resource_kind=$2 AND resource_id=$3::uuid AND epoch_id=$4::uuid AND end_revision IS NULL)`, c.Tenant, d.ResourceKind, d.ResourceID, d.EpochID).Scan(&current)
	return current, err
}
func (s *Store) ControlAuditCurrent(ctx context.Context, c *audit.AccessContext) (bool, error) {
	return controlAuditCurrent(ctx, s.Pool, c)
}
