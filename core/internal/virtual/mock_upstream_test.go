package virtual

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/bishoku/vmcp/core/internal/config"
	"github.com/bishoku/vmcp/core/internal/mcp"
	"github.com/bishoku/vmcp/core/internal/mockupstream"
	"github.com/bishoku/vmcp/core/internal/restapi"
)

func TestMockUpstreamThroughVirtualMCP(t *testing.T) {
	base := os.Getenv("VMCP_TEST_WIREMOCK_URL")
	if base == "" {
		if os.Getenv("VMCP_REQUIRE_INTEGRATION") == "1" {
			t.Fatal("VMCP_TEST_WIREMOCK_URL required")
		}
		t.Skip("VMCP_TEST_WIREMOCK_URL not set")
	}
	t.Setenv("VMCP_EGRESS_ALLOW_CIDRS", "127.0.0.0/8")
	owner := mockupstream.Owner(fmt.Sprint("virtual-mock-", time.Now().UnixNano()))
	client := mockupstream.NewClient(base)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := client.Reconcile(ctx, owner, nil); err != nil {
			t.Error(err)
		}
	})
	cfg := restapi.Config{Mock: &mockupstream.Config{Namespace: mockupstream.Namespace(owner, "orders")}, Auth: restapi.Auth{Type: "none"}, TimeoutSeconds: 3, AllowInsecureHTTP: true, Operations: []restapi.Operation{{Name: "create", Enabled: true, Method: "POST", Path: "/orders", BodySchema: json.RawMessage(`{"type":"object"}`), ResponseSchema: json.RawMessage(`{"type":"object","properties":{"id":{"type":"string"},"hidden":{"type":"string"}}}`), ResponseFields: []string{"id"}, MockRules: []mockupstream.Rule{{Name: "created", Priority: 1, Response: map[string]json.RawMessage{"status": json.RawMessage(`201`), "jsonBody": json.RawMessage(`{"id":"{{jsonPath request.body '$.order.id'}}","hidden":"not-for-agent"}`), "transformers": json.RawMessage(`["response-template"]`)}}}}}}
	compiled, err := restapi.Compile(base, cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := client.Reconcile(ctx, owner, mockupstream.Mappings(owner, *cfg.Mock, cfg.MockOperations())); err != nil {
		t.Fatal(err)
	}
	r := newRig(t)
	snap := r.handler.Snap.(staticSnap).s
	snap.Upstreams["orders"] = config.Upstream{ID: "orders", Slug: "orders", URL: base, REST: compiled}
	tool := compiled.Tools[0]
	tool.Name = "orders__create"
	snap.VirtualServers["support"] = config.VirtualServer{Slug: "support", Tools: []config.VirtualTool{{UpstreamID: "orders", UpstreamName: "create", Tool: tool}}}
	sid := r.initialize(t)
	_, listed := r.rpc(t, "support", sid, mcp.MethodToolsList, nil)
	if listed == nil || listed.Error != nil || strings.Contains(string(listed.Result), "response-template") {
		t.Fatalf("tools/list: %+v", listed)
	}
	_, called := r.rpc(t, "support", sid, mcp.MethodToolsCall, mcp.CallToolParams{Name: tool.Name, Arguments: json.RawMessage(`{"body":{"order":{"id":"ORD-42"}}}`)})
	if called == nil || called.Error != nil || strings.Contains(string(called.Result), `"isError":true`) || !strings.Contains(string(called.Result), "ORD-42") || strings.Contains(string(called.Result), "not-for-agent") {
		t.Fatalf("virtual mock result: %+v", called)
	}
}
