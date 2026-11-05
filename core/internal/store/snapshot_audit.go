package store

import (
	"context"
	"fmt"
	"github.com/bishoku/vmcp/core/internal/audit"
	"github.com/bishoku/vmcp/core/internal/config"
	"github.com/jackc/pgx/v5"
)

func loadAuditDomains(ctx context.Context, tx pgx.Tx, snap *config.Snapshot) error {
	rows, err := tx.Query(ctx, `SELECT e.resource_kind,e.resource_id::text,COALESCE(e.project_id::text,''),COALESCE(e.owner_team_id::text,''),e.epoch_id::text,COALESCE(v.slug,'') FROM audit_resource_epochs e LEFT JOIN virtual_servers v ON e.resource_kind='virtual_server' AND v.id=e.resource_id WHERE e.tenant_id=vmcp_tenant_id() AND e.end_revision IS NULL`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var d audit.AccessDomain
		var slug string
		if err = rows.Scan(&d.ResourceKind, &d.ResourceID, &d.ProjectID, &d.OwnerTeamID, &d.EpochID, &slug); err != nil {
			return err
		}
		if d.ResourceKind == "upstream" {
			if u, ok := snap.Upstreams[d.ResourceID]; ok {
				d.Kind = "provider"
				u.AuditDomain = d
				snap.Upstreams[d.ResourceID] = u
			}
		} else if v, ok := snap.VirtualServers[slug]; ok {
			d.Kind = "consumer"
			v.AuditDomain = d
			snap.VirtualServers[slug] = v
		}
	}
	if err = rows.Err(); err != nil {
		return err
	}
	for _, u := range snap.Upstreams {
		if u.AuditDomain.EpochID == "" {
			return fmt.Errorf("upstream snapshot lacks audit epoch")
		}
	}
	for _, v := range snap.VirtualServers {
		if v.AuditDomain.EpochID == "" {
			return fmt.Errorf("virtual server snapshot lacks audit epoch")
		}
	}
	return nil
}
