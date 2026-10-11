package store

import (
	"context"
	"encoding/json"
	"github.com/bishoku/vmcp/core/internal/config"
	"github.com/bishoku/vmcp/core/internal/mcp"
	"github.com/jackc/pgx/v5"
)

func loadVirtualCatalog(ctx context.Context, tx pgx.Tx, snap *config.Snapshot, slugs map[string]string) error {
	rows, err := tx.Query(ctx, `SELECT vc.virtual_server_id::text,vc.upstream_id::text,u.slug,vc.kind,vc.name,vc.alias,vc.description,uc.definition,u.mcp_capabilities FROM virtual_catalog vc JOIN upstreams u ON u.id=vc.upstream_id JOIN upstream_catalog uc ON uc.upstream_id=vc.upstream_id AND uc.kind=vc.kind AND uc.name=vc.name ORDER BY vc.virtual_server_id,vc.position`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var vsID, upID, upSlug, kind, name, alias, desc string
		var raw, capsRaw []byte
		if err := rows.Scan(&vsID, &upID, &upSlug, &kind, &name, &alias, &desc, &raw, &capsRaw); err != nil {
			return err
		}
		slug, ok := slugs[vsID]
		if !ok {
			continue
		}
		if _, ok := snap.Upstreams[upID]; !ok {
			continue
		}
		exposed := config.CatalogName(upSlug, kind, name, alias)
		o, err := config.CatalogDefinition(raw, kind, exposed, desc)
		if err != nil {
			return err
		}
		var caps mcp.ServerCapabilities
		_ = json.Unmarshal(capsRaw, &caps)
		v := snap.VirtualServers[slug]
		v.Catalog = append(v.Catalog, config.VirtualCatalogObject{UpstreamID: upID, Kind: kind, Original: name, Exposed: exposed, Definition: o, Subscribe: caps.Resources != nil && caps.Resources.Subscribe, Complete: caps.Completions != nil})
		snap.VirtualServers[slug] = v
	}
	return rows.Err()
}
