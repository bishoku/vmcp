package store

import (
	"context"
	"github.com/bishoku/vmcp/core/internal/config"
	"github.com/jackc/pgx/v5"
)

// applyOwnership runs at snapshot publication, not per tool invocation. Grant
// revocation uses the existing freshness and pre-dispatch authorization guards.
func applyOwnership(ctx context.Context, tx pgx.Tx, snap *config.Snapshot) error {
	rows, err := tx.Query(ctx, `SELECT id::text,owner_team_id::text FROM upstreams WHERE owner_team_id IS NOT NULL`)
	if err != nil {
		return err
	}
	owners := map[string]string{}
	for rows.Next() {
		var id, owner string
		if err := rows.Scan(&id, &owner); err != nil {
			rows.Close()
			return err
		}
		owners[id] = owner
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	if len(owners) == 0 {
		return nil
	}
	rows, err = tx.Query(ctx, `SELECT v.slug,COALESCE(p.owner_team_id::text,'') FROM virtual_servers v JOIN projects p ON p.id=v.project_id`)
	if err != nil {
		return err
	}
	projects := map[string]string{}
	for rows.Next() {
		var slug, owner string
		if err := rows.Scan(&slug, &owner); err != nil {
			rows.Close()
			return err
		}
		projects[slug] = owner
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	rows, err = tx.Query(ctx, `SELECT v.slug,g.upstream_id::text,g.tool_name,g.status FROM tool_grants g JOIN virtual_servers v ON v.id=g.virtual_server_id`)
	if err != nil {
		return err
	}
	grants := map[string]string{}
	for rows.Next() {
		var v, u, tool, status string
		if err := rows.Scan(&v, &u, &tool, &status); err != nil {
			rows.Close()
			return err
		}
		grants[v+"\x00"+u+"\x00"+tool] = status
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for slug, v := range snap.VirtualServers {
		tools := make([]config.VirtualTool, 0, len(v.Tools))
		for _, tool := range v.Tools {
			owner := owners[tool.UpstreamID]
			status := grants[slug+"\x00"+tool.UpstreamID+"\x00"+tool.UpstreamName]
			allowed := owner == "" || status == "approved" || (owner == projects[slug] && status != "revoked" && status != "rejected")
			if allowed {
				tools = append(tools, tool)
			} else {
				if v.Blocked == nil {
					v.Blocked = map[string]string{}
				}
				v.Blocked[tool.Tool.Name] = "upstream owner approval required"
			}
		}
		v.Tools = tools
		catalog := make([]config.VirtualCatalogObject, 0, len(v.Catalog))
		for _, c := range v.Catalog {
			owner := owners[c.UpstreamID]
			if owner == "" || owner == projects[slug] {
				catalog = append(catalog, c)
			}
		}
		v.Catalog = catalog
		allowedInstructions := []string{}
		for _, upSlug := range v.Guidance.UpstreamInstructions {
			for id, up := range snap.Upstreams {
				if up.Slug == upSlug && (owners[id] == "" || owners[id] == projects[slug]) {
					allowedInstructions = append(allowedInstructions, upSlug)
					break
				}
			}
		}
		v.Guidance.UpstreamInstructions = allowedInstructions
		snap.VirtualServers[slug] = v.WithToolIndex()
	}
	return nil
}
