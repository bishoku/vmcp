package store

import (
	"context"
	"errors"
	"strings"

	"github.com/bishoku/vmcp/core/internal/apierror"
	"github.com/jackc/pgx/v5"
)

type ToolGrant struct {
	CanDecide     bool   `json:"canDecide"`
	VirtualServer string `json:"virtualServer"`
	Upstream      string `json:"upstream"`
	Tool          string `json:"tool"`
	Status        string `json:"status"`
	Reason        string `json:"reason"`
	Actor         string `json:"actor"`
	Version       int64  `json:"version"`
}

func (t *Tx) ListToolGrants(ctx context.Context) ([]ToolGrant, error) {
	rows, err := t.q.Query(ctx, `SELECT v.slug,u.slug,g.tool_name,g.status,g.reason,g.actor,g.version,`+ownerPredicate(ctx, "u.owner_team_id")+` FROM tool_grants g JOIN virtual_servers v ON v.id=g.virtual_server_id JOIN upstreams u ON u.id=g.upstream_id WHERE (`+vsPredicate(ctx, "v")+` OR `+ownerPredicate(ctx, "u.owner_team_id")+`) ORDER BY v.slug,u.slug,g.tool_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ToolGrant{}
	for rows.Next() {
		var v ToolGrant
		if err := rows.Scan(&v.VirtualServer, &v.Upstream, &v.Tool, &v.Status, &v.Reason, &v.Actor, &v.Version, &v.CanDecide); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (t *Tx) PutToolGrant(ctx context.Context, g ToolGrant) error {
	if len(g.Reason) > 2000 || strings.TrimSpace(g.Tool) == "" {
		return apierror.Validation([]apierror.FieldError{vf("/tool", "tool required and reason limited to 2000 bytes")})
	}
	if g.Status != "pending" && g.Status != "approved" && g.Status != "rejected" && g.Status != "revoked" {
		return apierror.Validation([]apierror.FieldError{vf("/status", "must be pending, approved, rejected or revoked")})
	}
	var vsID, upID, upOwner, vsOwner string
	err := t.q.QueryRow(ctx, `SELECT v.id::text,u.id::text,COALESCE(ut.slug,''),COALESCE(pt.slug,'') FROM virtual_servers v JOIN projects p ON p.id=v.project_id CROSS JOIN upstreams u LEFT JOIN teams ut ON ut.id=u.owner_team_id LEFT JOIN teams pt ON pt.id=p.owner_team_id WHERE v.slug=$1 AND u.slug=$2 AND EXISTS(SELECT 1 FROM upstream_tools WHERE upstream_id=u.id AND name=$3)`, g.VirtualServer, g.Upstream, g.Tool).Scan(&vsID, &upID, &upOwner, &vsOwner)
	if errors.Is(err, pgx.ErrNoRows) {
		return apierror.NotFound("tool or virtual server", g.Tool)
	}
	if err != nil {
		return err
	}
	if g.Status == "pending" {
		if err = t.checkOwner(ctx, vsOwner); err != nil {
			return err
		}
		if upOwner == "" || upOwner == vsOwner {
			g.Status = "approved"
		}
	} else {
		if err = t.checkOwner(ctx, upOwner); err != nil {
			return err
		}
	}
	var version int64
	var status string
	err = t.q.QueryRow(ctx, `SELECT version,status FROM tool_grants WHERE virtual_server_id=$1::uuid AND upstream_id=$2::uuid AND tool_name=$3`, vsID, upID, g.Tool).Scan(&version, &status)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if err == nil {
		if version != g.Version {
			return apierror.PreconditionFailed()
		}
		if g.Status == "pending" && status == "approved" {
			return apierror.Forbidden("approved grants must be revoked by their upstream owner")
		}
	} else if g.Version != 0 {
		return apierror.PreconditionFailed()
	}
	a, _ := AccessFrom(ctx)
	g.Actor = a.Actor
	t.auditTarget("tool-grant", g.VirtualServer+"/"+g.Upstream+"/"+g.Tool)
	_, err = t.q.Exec(ctx, `INSERT INTO tool_grants(virtual_server_id,upstream_id,tool_name,status,reason,actor) VALUES($1::uuid,$2::uuid,$3,$4,$5,$6) ON CONFLICT(virtual_server_id,upstream_id,tool_name) DO UPDATE SET status=$4,reason=$5,actor=$6,version=tool_grants.version+1,updated_at=now()`, vsID, upID, g.Tool, g.Status, g.Reason, g.Actor)
	return err
}

// ToolCatalog exposes only discoverable metadata, never connection details.
type ToolCatalogEntry struct {
	Upstream    string `json:"upstream"`
	OwnerTeam   string `json:"ownerTeam"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (t *Tx) ToolCatalog(ctx context.Context, after string, limit int) ([]ToolCatalogEntry, bool, error) {
	rows, err := t.q.Query(ctx, `SELECT u.slug,COALESCE(t.slug,''),ut.name,ut.description FROM upstream_tools ut JOIN upstreams u ON u.id=ut.upstream_id LEFT JOIN teams t ON t.id=u.owner_team_id WHERE u.enabled AND (u.slug||'/'||ut.name)>$1 ORDER BY u.slug,ut.name LIMIT $2`, after, limit+1)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	out := []ToolCatalogEntry{}
	for rows.Next() {
		var v ToolCatalogEntry
		if err := rows.Scan(&v.Upstream, &v.OwnerTeam, &v.Name, &v.Description); err != nil {
			return nil, false, err
		}
		out = append(out, v)
	}
	more := len(out) > limit
	if more {
		out = out[:limit]
	}
	return out, more, rows.Err()
}
