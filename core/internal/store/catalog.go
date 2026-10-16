package store

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/bishoku/vmcp/core/internal/apierror"
	"github.com/bishoku/vmcp/core/internal/config"
	"github.com/bishoku/vmcp/core/internal/mcp"
)

type CatalogSelection struct {
	Upstream    string `json:"upstream"`
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	Alias       string `json:"alias,omitempty"`
	Description string `json:"description,omitempty"`
}
type ResolvedCatalogSelection struct {
	CatalogSelection
	ExposedName         string `json:"exposedName"`
	Orphaned            bool   `json:"orphaned"`
	DescriptionDrift    bool   `json:"descriptionDrift"`
	UpstreamDescription string `json:"upstreamDescription"`
}

func (t *Tx) ReplaceCatalog(ctx context.Context, slug string, catalog mcp.Catalog) error {
	for _, family := range []struct {
		objects []mcp.CatalogObject
		key     string
	}{{catalog.Resources, "uri"}, {catalog.Templates, "uriTemplate"}, {catalog.Prompts, "name"}} {
		sort.Slice(family.objects, func(i, j int) bool {
			return family.objects[i].String(family.key) < family.objects[j].String(family.key)
		})
	}
	var id string
	if err := t.q.QueryRow(ctx, `SELECT id::text FROM upstreams WHERE tenant_id=`+tenantSQL+` AND slug=$1`, slug).Scan(&id); err != nil {
		return err
	}
	current, err := t.ListCatalog(ctx, slug)
	if err != nil {
		return err
	}
	caps, err := json.Marshal(catalog.Capabilities)
	if err != nil {
		return err
	}
	var oldCaps []byte
	if err := t.q.QueryRow(ctx, `SELECT mcp_capabilities FROM upstreams WHERE id=$1`, id).Scan(&oldCaps); err != nil {
		return err
	}
	var old mcp.ServerCapabilities
	_ = json.Unmarshal(oldCaps, &old)
	if reflect.DeepEqual(current.Resources, catalog.Resources) && reflect.DeepEqual(current.Templates, catalog.Templates) && reflect.DeepEqual(current.Prompts, catalog.Prompts) && reflect.DeepEqual(old, catalog.Capabilities) {
		return nil
	}
	if _, err := t.q.Exec(ctx, `DELETE FROM upstream_catalog WHERE upstream_id=$1`, id); err != nil {
		return err
	}
	var kinds, names, defs []string
	for _, f := range []struct {
		kind, key string
		objects   []mcp.CatalogObject
	}{{"resource", "uri", catalog.Resources}, {"template", "uriTemplate", catalog.Templates}, {"prompt", "name", catalog.Prompts}} {
		for _, o := range f.objects {
			raw, err := json.Marshal(o)
			if err != nil {
				return err
			}
			kinds = append(kinds, f.kind)
			names = append(names, o.String(f.key))
			defs = append(defs, string(raw))
		}
	}
	if len(kinds) > 0 {
		if _, err := t.q.Exec(ctx, `
			INSERT INTO upstream_catalog(upstream_id, kind, name, definition)
			SELECT $1::uuid, u.kind, u.name, u.definition::jsonb
			FROM UNNEST($2::text[], $3::text[], $4::text[]) AS u(kind, name, definition)`,
			id, kinds, names, defs); err != nil {
			return err
		}
	}
	_, err = t.q.Exec(ctx, `UPDATE upstreams SET mcp_capabilities=$2 WHERE id=$1`, id, caps)
	return err
}
func (t *Tx) ListCatalog(ctx context.Context, slug string) (mcp.Catalog, error) {
	out := mcp.Catalog{Resources: []mcp.CatalogObject{}, Templates: []mcp.CatalogObject{}, Prompts: []mcp.CatalogObject{}}
	var raw []byte
	if err := t.q.QueryRow(ctx, `SELECT mcp_capabilities FROM upstreams WHERE tenant_id=`+tenantSQL+` AND slug=$1`, slug).Scan(&raw); err != nil {
		return out, err
	}
	if err := json.Unmarshal(raw, &out.Capabilities); err != nil {
		return out, err
	}
	rows, err := t.q.Query(ctx, `SELECT c.kind,c.definition FROM upstream_catalog c JOIN upstreams u ON u.id=c.upstream_id WHERE u.tenant_id=`+tenantSQL+` AND u.slug=$1 ORDER BY c.kind,c.name`, slug)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var kind string
		var raw []byte
		if err := rows.Scan(&kind, &raw); err != nil {
			return out, err
		}
		var o mcp.CatalogObject
		if err := json.Unmarshal(raw, &o); err != nil {
			return out, err
		}
		switch kind {
		case "resource":
			out.Resources = append(out.Resources, o)
		case "template":
			out.Templates = append(out.Templates, o)
		case "prompt":
			out.Prompts = append(out.Prompts, o)
		}
	}
	return out, rows.Err()
}
func (t *Tx) loadCatalogSelections(ctx context.Context, all map[string]*vsRow) error {
	rows, err := t.q.Query(ctx, `SELECT vc.virtual_server_id::text,u.slug,vc.kind,vc.name,vc.alias,vc.description,vc.description_basis,uc.definition FROM virtual_catalog vc JOIN upstreams u ON u.id=vc.upstream_id LEFT JOIN upstream_catalog uc ON uc.upstream_id=vc.upstream_id AND uc.kind=vc.kind AND uc.name=vc.name ORDER BY vc.virtual_server_id,vc.position`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, basis string
		var c ResolvedCatalogSelection
		var raw []byte
		if err := rows.Scan(&id, &c.Upstream, &c.Kind, &c.Name, &c.Alias, &c.Description, &basis, &raw); err != nil {
			return err
		}
		v := all[id]
		if v == nil {
			continue
		}
		c.Orphaned = raw == nil
		var o mcp.CatalogObject
		_ = json.Unmarshal(raw, &o)
		c.UpstreamDescription = o.String("description")
		c.DescriptionDrift = c.Description != "" && basis != c.UpstreamDescription
		c.ExposedName = config.CatalogName(c.Upstream, c.Kind, c.Name, c.Alias)
		v.Catalog = append(v.Catalog, c)
	}
	return rows.Err()
}
func (t *Tx) writeCatalogSelections(ctx context.Context, id string, sels []CatalogSelection) error {
	if len(sels) > 10000 {
		return apierror.Validation([]apierror.FieldError{{Field: "/catalog", Code: apierror.CodeValidationFailed, Message: "at most 10000 selections"}})
	}
	seen := map[string]bool{}
	for i, c := range sels {
		field := fmt.Sprintf("/catalog/%d", i)
		if c.Kind != "resource" && c.Kind != "template" && c.Kind != "prompt" {
			return apierror.Validation([]apierror.FieldError{{Field: field + "/kind", Code: apierror.CodeValidationFailed, Message: "must be resource, template or prompt"}})
		}
		if (c.Kind == "template" && c.Alias != "") || c.Name == "" || len(c.Name) > 4096 || len(c.Alias) > 128 || (c.Alias != "" && strings.ContainsAny(c.Alias, " /?#{}")) {
			return apierror.Validation([]apierror.FieldError{{Field: field, Code: apierror.CodeValidationFailed, Message: "invalid catalog identity or alias"}})
		}
		if errs := validateCapabilityDescription(field+"/description", c.Description); len(errs) > 0 {
			return apierror.Validation(errs)
		}
		exposed := c.Kind + ":" + config.CatalogName(c.Upstream, c.Kind, c.Name, c.Alias)
		if seen[exposed] {
			return apierror.Validation([]apierror.FieldError{{Field: field, Code: apierror.CodeValidationFailed, Message: "duplicate exposed catalog identity"}})
		}
		seen[exposed] = true
		var exists bool
		if err := t.q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM upstream_catalog uc JOIN upstreams u ON u.id=uc.upstream_id WHERE u.tenant_id=`+tenantSQL+` AND u.slug=$1 AND uc.kind=$2 AND uc.name=$3) OR EXISTS(SELECT 1 FROM virtual_catalog vc JOIN upstreams u ON u.id=vc.upstream_id WHERE vc.virtual_server_id=$4 AND u.slug=$1 AND vc.kind=$2 AND vc.name=$3)`, c.Upstream, c.Kind, c.Name, id).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return apierror.Validation([]apierror.FieldError{{Field: field, Code: apierror.CodeValidationFailed, Message: "catalog object is not discovered"}})
		}
	}
	if _, err := t.q.Exec(ctx, `DELETE FROM virtual_catalog WHERE virtual_server_id=$1`, id); err != nil {
		return err
	}
	for i, c := range sels {
		if _, err := t.q.Exec(ctx, `INSERT INTO virtual_catalog(virtual_server_id,upstream_id,kind,name,alias,description,description_basis,position) SELECT $1,u.id,$3,$4,$5,$6,COALESCE((SELECT definition->>'description' FROM upstream_catalog WHERE upstream_id=u.id AND kind=$3 AND name=$4),''),$7 FROM upstreams u WHERE u.tenant_id=`+tenantSQL+` AND u.slug=$2`, id, c.Upstream, c.Kind, c.Name, c.Alias, c.Description, i); err != nil {
			return err
		}
	}
	return nil
}
func sameCatalogSelections(current []ResolvedCatalogSelection, next []CatalogSelection) bool {
	if len(current) != len(next) {
		return false
	}
	for i, c := range current {
		if c.CatalogSelection != next[i] {
			return false
		}
	}
	return true
}
