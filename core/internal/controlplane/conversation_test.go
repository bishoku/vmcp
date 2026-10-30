package controlplane_test

import (
	"context"
	"fmt"
	"github.com/bishoku/vmcp/core/internal/store"
	"testing"
)

func TestAgentKeyCorrelationSettingsAndSnapshot(t *testing.T) {
	a := setup(t)
	mapping := map[string]any{"conversationId": "/client~1conversation", "rootConversationId": "/nested/root"}
	_, out := a.expect(201, "POST", "/api/v1/agent-keys", map[string]any{"slug": "correlated", "auditCorrelation": mapping})
	if out["auditCorrelation"].(map[string]any)["conversationId"] != "/client~1conversation" {
		t.Fatal(out)
	}
	a.expect(422, "PATCH", "/api/v1/agent-keys/correlated", map[string]any{"auditCorrelation": map[string]any{"conversationId": "invalid"}})
	_, out = a.expect(200, "PATCH", "/api/v1/agent-keys/correlated", map[string]any{"description": "keep mapping"})
	if len(out["auditCorrelation"].(map[string]any)) != 2 {
		t.Fatal("omission cleared mapping")
	}
	_, out = a.expect(200, "POST", "/api/v1/agent-keys/correlated/rotate", nil)
	snap, err := store.LoadSnapshot(context.Background(), a.pool, nil)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, k := range snap.AgentKeys {
		if k.Slug == "correlated" {
			found = k.AuditCorrelation.ConversationID == "/client~1conversation"
		}
	}
	if !found {
		t.Fatal("snapshot mapping missing after rotation")
	}
	_, out = a.expect(200, "PATCH", "/api/v1/agent-keys/correlated", map[string]any{"auditCorrelation": map[string]any{}})
	if len(out["auditCorrelation"].(map[string]any)) != 0 {
		t.Fatal("explicit empty did not clear")
	}
}
func TestAuditConversationGroupingFiltersAndPagination(t *testing.T) {
	a := setup(t)
	ctx := context.Background()
	for i := 1; i <= 5; i++ {
		group := "group-a"
		actor := "key-a"
		if i == 5 {
			group = "group-b"
			actor = "key-b"
		}
		parent := ""
		if i == 3 {
			parent = "40000000-0000-4000-8000-000000000001"
		}
		stage := "outcome"
		status := "ok"
		op := i
		if i == 2 {
			op = 1
			stage = "admission"
			status = "started"
		}
		_, err := a.pool.Exec(ctx, `INSERT INTO audit_events(virtual_server,method,operation_id,request_id,parent_operation_id,stage,status,actor_id,conversation_id,root_conversation_id,conversation_group_id,turn_id)VALUES('','tools/call',$1::uuid,$1::uuid,NULLIF($2,'')::uuid,$3,$4,$5,'child','root',$6,'turn')`, fmt.Sprintf("40000000-0000-4000-8000-%012d", op), parent, stage, status, actor, group)
		if err != nil {
			t.Fatal(err)
		}
	}
	_, first := a.expect(200, "GET", "/api/v1/audit-events?groupBy=conversation&limit=1", nil)
	item := first["items"].([]any)[0].(map[string]any)
	if item["conversationGroupId"] != "group-b" {
		t.Fatal(first)
	}
	_, second := a.expect(200, "GET", fmt.Sprint("/api/v1/audit-events?groupBy=conversation&limit=1&cursor=", first["nextCursor"]), nil)
	item = second["items"].([]any)[0].(map[string]any)
	if item["records"] != float64(4) || item["operations"] != float64(2) || second["nextCursor"] != nil {
		t.Fatal(second)
	}
	_, filtered := a.expect(200, "GET", "/api/v1/audit-events?conversationGroupId=group-a&turnId=turn", nil)
	if len(filtered["items"].([]any)) != 4 {
		t.Fatal(filtered)
	}
	_, filtered = a.expect(200, "GET", "/api/v1/audit-events?groupBy=conversation&actorId=key-b", nil)
	if len(filtered["items"].([]any)) != 1 {
		t.Fatal(filtered)
	}
}

func TestConversationSummaryDoesNotBroadenAuditVisibility(t *testing.T) {
	a := setup(t)
	ctx := context.Background()
	if _, err := a.pool.Exec(ctx, `INSERT INTO audit_events(virtual_server,method,status,actor_id,conversation_id,conversation_group_id)VALUES('','tools/call','ok','allowed','same','scoped'),('','tools/call','ok','other','same','scoped')`); err != nil {
		t.Fatal(err)
	}
	reader := (&store.Store{Pool: a.pool}).Reader()
	limited := store.WithAccess(ctx, store.Access{Role: "viewer", Actor: "allowed"})
	groups, _, err := reader.ListAuditConversations(limited, store.AuditFilter{}, 100)
	if err != nil || len(groups) != 1 || groups[0].Records != 1 {
		t.Fatalf("summary leaked records: %+v %v", groups, err)
	}
	events, _, err := reader.ListAudit(limited, store.AuditFilter{ConversationGroupID: "scoped"}, 100)
	if err != nil || len(events) != 1 {
		t.Fatalf("drilldown leaked records: %+v %v", events, err)
	}
}
