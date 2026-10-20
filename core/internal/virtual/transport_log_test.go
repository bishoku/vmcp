package virtual

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/bishoku/vmcp/core/internal/config"
	"github.com/bishoku/vmcp/core/internal/mcp"
	"github.com/bishoku/vmcp/core/internal/restapi"
)

func TestRESTFailureLogsSafeCauseAndKeepsClientErrorGeneric(t *testing.T) {
	t.Setenv("VMCP_EGRESS_ALLOW_HOSTS", "internal.example")
	rig := newRig(t)
	compiled, err := restapi.Compile("https://httpbin.org", restapi.Config{
		Auth: restapi.Auth{Type: "none"}, TimeoutSeconds: 3,
		Operations: []restapi.Operation{{
			Name: "create_issue", Enabled: true, Method: "GET", Path: "/ip",
			Parameters: []restapi.Parameter{{Name: "q", In: "query", Schema: json.RawMessage(`{"type":"string"}`)}},
		}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	rig.handler.Snap.Snapshot().Upstreams["u-gh"] = config.Upstream{
		ID: "u-gh", Slug: "httpbin", URL: "https://httpbin.org", REST: compiled,
		Headers: map[string]string{"X-Key": "secret-header"},
	}
	var logs bytes.Buffer
	rig.handler.Log = slog.New(slog.NewJSONHandler(&logs, nil))
	sid := rig.initialize(t)
	_, response := rig.rpc(t, "support", sid, mcp.MethodToolsCall,
		mcp.CallToolParams{Name: "github__create_issue", Arguments: json.RawMessage(`{"q":"secret-query"}`)})
	if response == nil || response.Error == nil || response.Error.Message != "upstream unavailable" {
		t.Fatalf("unexpected client response: %+v", response)
	}
	var record map[string]any
	if err := json.Unmarshal(logs.Bytes(), &record); err != nil {
		t.Fatal(err)
	}
	if record["reason"] != "egress_host_not_allowed" || record["stage"] != "request" || record["virtual_server"] != "support" || record["upstream"] != "httpbin" || record["duration_ms"] == nil {
		t.Fatalf("missing diagnostic fields: %s", logs.String())
	}
	if strings.Contains(logs.String(), "secret-") || strings.Contains(logs.String(), "/ip") {
		t.Fatal("request contents leaked into logs")
	}
}
