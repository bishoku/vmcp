package controlplane_test

import (
	"context"
	"fmt"
	"testing"
)

func TestAuditActivityFilterKeepsLinkedAttemptsAndPagination(t *testing.T) {
	a := setup(t)
	ctx := context.Background()
	virtual := "10000000-0000-4000-8000-000000000001"
	control := "10000000-0000-4000-8000-000000000002"
	// Deliberately reuse the server, trace and request identities across origins.
	// Those fields must never turn administrative or system activity into MCP calls.
	rows := []struct{ scope, operation, parent, tool string }{
		{"virtual", virtual, "", "virtual-root"},
		{"upstream", "10000000-0000-4000-8000-000000000003", virtual, "virtual-child"},
		{"control", control, "", "control-root"},
		{"reads", "10000000-0000-4000-8000-000000000004", "", "control-read"},
		{"upstream", "10000000-0000-4000-8000-000000000005", control, "control-child"},
		{"system", "10000000-0000-4000-8000-000000000006", "", "system"},
		{"upstream", "10000000-0000-4000-8000-000000000007", "", "orphan-upstream"},
		{"", "10000000-0000-4000-8000-000000000008", "", "legacy"},
	}
	for _, row := range rows {
		_, err := a.pool.Exec(ctx, `INSERT INTO audit_events(scope,operation_id,parent_operation_id,virtual_server,method,tool,status,stage,trace_id,request_id) VALUES($1,$2::uuid,NULLIF($3,'')::uuid,'same-server','tools/call',$4,'ok','outcome','same-trace','20000000-0000-4000-8000-000000000001'::uuid)`, row.scope, row.operation, row.parent, row.tool)
		if err != nil {
			t.Fatal(err)
		}
	}
	for activity, want := range map[string][]string{
		"virtual":  {"virtual-child", "virtual-root"},
		"control":  {"control-child", "control-read", "control-root"},
		"system":   {"system"},
		"upstream": {"orphan-upstream", "control-child", "virtual-child"},
		"":         {"legacy", "orphan-upstream", "system", "control-child", "control-read", "control-root", "virtual-child", "virtual-root"},
	} {
		t.Run("activity="+activity, func(t *testing.T) {
			_, out := a.expect(200, "GET", "/api/v1/audit-events?activity="+activity+"&virtualServer=same-server", nil)
			items := out["items"].([]any)
			if len(items) != len(want) {
				t.Fatalf("wanted %v, got %v", want, items)
			}
			for i, tool := range want {
				if items[i].(map[string]any)["tool"] != tool {
					t.Fatalf("wanted %v, got %v", want, items)
				}
			}
		})
	}
	_, first := a.expect(200, "GET", "/api/v1/audit-events?activity=virtual&limit=1", nil)
	_, second := a.expect(200, "GET", fmt.Sprint("/api/v1/audit-events?activity=virtual&limit=1&cursor=", first["nextCursor"]), nil)
	if first["items"].([]any)[0].(map[string]any)["tool"] != "virtual-child" || second["items"].([]any)[0].(map[string]any)["tool"] != "virtual-root" || second["nextCursor"] != nil {
		t.Fatalf("pagination split membership incorrectly: %v / %v", first, second)
	}
	_, filtered := a.expect(200, "GET", "/api/v1/audit-events?activity=virtual&tool=virtual-child&operationId="+virtual, nil)
	if len(filtered["items"].([]any)) != 1 {
		t.Fatalf("combined filters: %v", filtered)
	}
	a.expect(422, "GET", "/api/v1/audit-events?activity=unknown", nil)
}

func TestOmniActivityIncludesOnlyExplicitDescendants(t *testing.T) {
	a := setup(t)
	ctx := context.Background()
	root := "30000000-0000-4000-8000-000000000001"
	rows := []struct{ scope, id, parent string }{
		{"omni", root, ""}, {"omni_diagnostics", "30000000-0000-4000-8000-000000000002", root}, {"virtual", "30000000-0000-4000-8000-000000000003", root},
		{"upstream", "30000000-0000-4000-8000-000000000004", "30000000-0000-4000-8000-000000000003"},
		{"virtual", "30000000-0000-4000-8000-000000000005", ""},
	}
	for _, row := range rows {
		if _, err := a.pool.Exec(ctx, `INSERT INTO audit_events(scope,operation_id,parent_operation_id,virtual_server,method,status,stage,request_id)VALUES($1,$2::uuid,NULLIF($3,'')::uuid,'','tools/call','ok','outcome','30000000-0000-4000-8000-000000000001'::uuid)`, row.scope, row.id, row.parent); err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range []string{"/api/v1/audit-events?activity=omni", "/api/v1/audit-events?activity=omni&operationId=" + root} {
		result := a.want("GET", path, nil, 200)
		if len(result["items"].([]any)) != 4 {
			t.Fatalf("Omni descendants: %v", result)
		}
	}
}
