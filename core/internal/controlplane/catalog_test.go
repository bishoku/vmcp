package controlplane_test

import (
	"github.com/bishoku/vmcp/core/internal/fixture"
	"net/http/httptest"
	"testing"
)

func TestCatalogDiscoveryCurationGitOpsAndOrphans(t *testing.T) {
	a := setup(t)
	physical := httptest.NewServer(fixture.NewCatalog())
	defer physical.Close()
	for _, slug := range []string{"catalog-a", "catalog-b"} {
		status, _, body := a.do("POST", "/api/v1/upstreams", map[string]any{"slug": slug, "name": slug, "url": physical.URL}, nil)
		if status != 201 {
			t.Fatalf("create %d %+v", status, body)
		}
		status, _, body = a.do("POST", "/api/v1/upstreams/"+slug+"/refresh-tools", nil, nil)
		if status != 200 {
			t.Fatalf("discovery %d %+v", status, body)
		}
	}
	status, _, catalog := a.do("GET", "/api/v1/upstreams/catalog-a/catalog", nil, nil)
	if status != 200 || len(catalog["resources"].([]any)) != 3 || len(catalog["prompts"].([]any)) != 1 {
		t.Fatalf("catalog %d %+v", status, catalog)
	}
	selections := []any{map[string]any{"upstream": "catalog-a", "kind": "resource", "name": "doc://public"}, map[string]any{"upstream": "catalog-b", "kind": "resource", "name": "doc://public"}, map[string]any{"upstream": "catalog-a", "kind": "template", "name": "doc://items/{id}"}, map[string]any{"upstream": "catalog-a", "kind": "prompt", "name": "review", "alias": "review_a", "description": "Approved description"}}
	body := map[string]any{"slug": "catalog-agent", "name": "Catalog Agent", "authRequired": false, "catalog": selections}
	status, headers, vs := a.do("POST", "/api/v1/virtual-servers", body, nil)
	if status != 201 {
		t.Fatalf("curation %d %+v", status, vs)
	}
	if status, _, _ := a.do("DELETE", "/api/v1/upstreams/catalog-a", nil, nil); status != 409 {
		t.Fatalf("selected catalog upstream deleted: %d", status)
	}
	rows := vs["catalog"].([]any)
	if rows[0].(map[string]any)["exposedName"] == rows[1].(map[string]any)["exposedName"] {
		t.Fatal("colliding upstream URI was not namespaced")
	}
	if status, _, _ := a.do("PATCH", "/api/v1/virtual-servers/catalog-agent", map[string]any{"catalog": []any{map[string]string{"upstream": "catalog-a", "kind": "resource", "name": "doc://absent"}}}, map[string]string{"If-Match": headers.Get("ETag")}); status != 422 {
		t.Fatalf("undiscovered resource accepted: %d", status)
	}
	_, _, exported := a.do("GET", "/api/v1/export", nil, nil)
	if status, _, result := a.do("POST", "/api/v1/apply?dryRun=true&refreshTools=false", exported, nil); status != 200 {
		t.Fatalf("catalog dry run %d %+v", status, result)
	}
	if status, _, result := a.do("POST", "/api/v1/apply?refreshTools=false", exported, nil); status != 200 {
		t.Fatalf("catalog roundtrip %d %+v", status, result)
	}
	if _, err := a.pool.Exec(t.Context(), `UPDATE upstream_catalog SET definition=jsonb_set(definition,'{description}','"changed upstream description"') WHERE kind='prompt';DELETE FROM upstream_catalog WHERE kind='resource' AND name='doc://public'`); err != nil {
		t.Fatal(err)
	}
	_, _, vs = a.do("GET", "/api/v1/virtual-servers/catalog-agent", nil, nil)
	rows = vs["catalog"].([]any)
	if rows[0].(map[string]any)["orphaned"] != true || rows[3].(map[string]any)["descriptionDrift"] != true {
		t.Fatalf("orphan/drift hidden: %+v", rows)
	}
	_, _, exported = a.do("GET", "/api/v1/export", nil, nil)
	if status, _, result := a.do("POST", "/api/v1/apply?refreshTools=false", exported, nil); status != 200 {
		t.Fatalf("existing orphan roundtrip %d %+v", status, result)
	}
}
