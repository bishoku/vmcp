package store

import (
	"context"

	"github.com/bishoku/vmcp/core/internal/audit"
)

// ScopeOption includes immutable IDs needed for assignments, never credentials.
type ScopeOption struct {
	ID        string `json:"id"`
	Slug      string `json:"slug"`
	Name      string `json:"name"`
	OwnerTeam string `json:"ownerTeam,omitempty"`
	Project   string `json:"project,omitempty"`
	Epoch     string `json:"epoch,omitempty"`
}

func (t *Tx) AuthorizationScopeOptions(ctx context.Context, kind, after string, limit int) ([]ScopeOption, bool, error) {
	if err := authorizationAdmin(ctx); err != nil {
		return nil, false, err
	}
	query := ""
	switch kind {
	case "team":
		query = `SELECT id::text,slug,name,'','','' FROM teams`
	case "project":
		query = `SELECT id::text,slug,name,COALESCE(owner_team_id::text,''),'','' FROM projects`
	case "control_token":
		query = `SELECT id::text,slug,slug,'','','' FROM admin_tokens WHERE revoked_at IS NULL AND (expires_at IS NULL OR expires_at>now())`
	case "upstream", "virtual_server":
		table := "upstreams"
		if kind == "virtual_server" {
			table = "virtual_servers"
		}
		query = `SELECT r.id::text,r.slug,r.name,COALESCE(e.owner_team_id::text,''),COALESCE(e.project_id::text,''),e.epoch_id::text FROM ` + table + ` r JOIN audit_resource_epochs e ON e.resource_id=r.id AND e.resource_kind='` + kind + `' AND e.end_revision IS NULL AND e.tenant_id=vmcp_tenant_id()`
	default:
		return nil, false, roleValidation("/kind", "use team, project, upstream, virtual_server or control_token")
	}
	rows, err := t.q.Query(ctx, `SELECT * FROM (`+query+`) options WHERE id>$1 ORDER BY id LIMIT $2`, after, limit+1)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	items := []ScopeOption{}
	for rows.Next() {
		var v ScopeOption
		if err := rows.Scan(&v.ID, &v.Slug, &v.Name, &v.OwnerTeam, &v.Project, &v.Epoch); err != nil {
			return nil, false, err
		}
		items = append(items, v)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	more := len(items) > limit
	if more {
		items = items[:limit]
	}
	return items, more, nil
}

type ResourceEpochOption struct {
	ID            string `json:"id"`
	ResourceID    string `json:"resourceId"`
	Kind          string `json:"kind"`
	OwnerTeam     string `json:"ownerTeam,omitempty"`
	Project       string `json:"project,omitempty"`
	StartRevision int64  `json:"startRevision"`
	EndRevision   *int64 `json:"endRevision"`
}

func (t *Tx) AuthorizationResourceEpochs(ctx context.Context, kind, id, after string, limit int) ([]ResourceEpochOption, bool, error) {
	if err := authorizationAdmin(ctx); err != nil {
		return nil, false, err
	}
	if (kind != "upstream" && kind != "virtual_server") || !audit.ValidID(id) {
		return nil, false, roleValidation("/resourceId", "resource kind and UUID are required")
	}
	rows, err := t.q.Query(ctx, `SELECT epoch_id::text,resource_id::text,resource_kind,COALESCE(owner_team_id::text,''),COALESCE(project_id::text,''),start_revision,end_revision FROM audit_resource_epochs WHERE tenant_id=vmcp_tenant_id() AND resource_kind=$1 AND resource_id=$2::uuid AND epoch_id::text>$3 ORDER BY epoch_id::text LIMIT $4`, kind, id, after, limit+1)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	items := []ResourceEpochOption{}
	for rows.Next() {
		var v ResourceEpochOption
		if err := rows.Scan(&v.ID, &v.ResourceID, &v.Kind, &v.OwnerTeam, &v.Project, &v.StartRevision, &v.EndRevision); err != nil {
			return nil, false, err
		}
		items = append(items, v)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	more := len(items) > limit
	if more {
		items = items[:limit]
	}
	return items, more, nil
}
