package virtual

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bishoku/vmcp/core/internal/mcp"
	"github.com/bishoku/vmcp/core/internal/telemetry"
)

func TestMetricsUseConfiguredAliasesAndOriginalUpstreamTool(t *testing.T) {
	r := newRig(t)
	metrics := telemetry.New()
	r.handler.Metrics = metrics
	sid := r.initialize(t)
	r.rpc(t, "support", sid, mcp.MethodToolsCall, mcp.CallToolParams{Name: "tables", Arguments: json.RawMessage(`{"secret":"do-not-export"}`)})
	vs := r.handler.Snap.Snapshot().VirtualServers["support"]
	vs.Blocked = map[string]string{"blocked": "policy deny"}
	r.handler.Snap.Snapshot().VirtualServers["support"] = vs
	r.rpc(t, "support", sid, mcp.MethodToolsCall, mcp.CallToolParams{Name: "blocked"})
	// Arbitrary supplied tool/server names must never become label sets.
	r.rpc(t, "support", sid, mcp.MethodToolsCall, mcp.CallToolParams{Name: "unconfigured-secret"})
	r.rpc(t, "unknown-secret", sid, mcp.MethodToolsList, nil)
	r.postgres.mu.Lock()
	r.postgres.failCall = true
	r.postgres.mu.Unlock()
	r.rpc(t, "support", sid, mcp.MethodToolsCall, mcp.CallToolParams{Name: "tables"})
	out := httptest.NewRecorder()
	metrics.ServeHTTP(out, httptest.NewRequest("GET", "/metrics", nil))
	body := out.Body.String()
	for _, want := range []string{
		`vmcp_virtual_tool_calls_total{virtual_server="support",tool="tables",outcome="ok"} 1`,
		`vmcp_virtual_tool_calls_total{virtual_server="support",tool="blocked",outcome="denied"} 1`,
		`vmcp_virtual_tool_calls_total{virtual_server="support",tool="tables",outcome="error"} 1`,
		`vmcp_upstream_requests_total{upstream="postgres",method="tools/call",tool="list_tables",outcome="ok"} 1`,
		`vmcp_upstream_requests_total{upstream="postgres",method="tools/call",tool="list_tables",outcome="error"} 1`,
		`vmcp_virtual_requests_total{virtual_server="support",outcome="ok"}`,
	} {
		if !strings.Contains(body, want) {
			t.Fatal(want, body)
		}
	}
	if strings.Contains(body, "secret") || strings.Contains(body, "do-not-export") {
		t.Fatal("untrusted values became metric labels", body)
	}
}
func TestOperationOutcome(t *testing.T) {
	if operationOutcome(context.Background(), json.RawMessage(`{"isError":true}`), nil) != "tool_error" {
		t.Fatal("MCP tool error counted as success")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if operationOutcome(ctx, nil, errors.New("private error")) != "cancelled" {
		t.Fatal("cancellation outcome")
	}
}
func TestMetricsCountGatewayRetriesSeparately(t *testing.T) {
	r := newRig(t)
	metrics := telemetry.New()
	r.handler.Metrics = metrics
	failed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(503) }))
	defer failed.Close()
	snap := r.handler.Snap.Snapshot()
	up := snap.Upstreams["u-gh"]
	up.URL = failed.URL
	snap.Upstreams["u-gh"] = up
	vs := snap.VirtualServers["support"]
	vs.Tools[0].Tool.Annotations = json.RawMessage(`{"readOnlyHint":true}`)
	snap.VirtualServers["support"] = vs
	sid := r.initialize(t)
	_, reply := r.rpc(t, "support", sid, mcp.MethodToolsCall, mcp.CallToolParams{Name: "github__create_issue"})
	if reply.Error == nil {
		t.Fatal("failed upstream succeeded")
	}
	out := httptest.NewRecorder()
	metrics.ServeHTTP(out, httptest.NewRequest("GET", "/metrics", nil))
	body := out.Body.String()
	for _, want := range []string{`vmcp_upstream_requests_total{upstream="github",method="tools/call",tool="create_issue",outcome="error"} 2`, `vmcp_virtual_tool_calls_total{virtual_server="support",tool="github__create_issue",outcome="error"} 1`} {
		if !strings.Contains(body, want) {
			t.Fatal(want, body)
		}
	}
}
