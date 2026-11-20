package mcp

import (
	"net/http"
	"testing"
)

func TestModernTransportMirrors(t *testing.T) {
	t.Setenv("VMCP_MCP_20260728", "1")
	newCall := func() *Message {
		msg, err := NewRequest(NewIntID(3), MethodToolsCall, map[string]any{"name": "öz", "arguments": map[string]any{}, "_meta": map[string]any{"io.modelcontextprotocol/protocolVersion": ProtocolVersion20260728, "io.modelcontextprotocol/clientCapabilities": map[string]any{}, "traceparent": "abc"}})
		if err != nil {
			t.Fatal(err)
		}
		return msg
	}
	headers := http.Header{}
	headers.Set("Mcp-Protocol-Version", ProtocolVersion20260728)
	headers.Set("Mcp-Method", MethodToolsCall)
	headers.Set("Mcp-Name", "=?base64?w7Z6?=")
	if err := ValidateModern(headers, newCall()); err != nil {
		t.Fatal(err.Error)
	}
	for _, name := range []string{"Mcp-Method", "Mcp-Name", "Mcp-Protocol-Version"} {
		h := headers.Clone()
		h.Del(name)
		msg := newCall()
		if !ModernRequest(h, msg) {
			t.Fatalf("missing %s routed as legacy", name)
		}
		if invalid := ValidateModern(h, msg); invalid == nil || invalid.Error.Code != -32020 {
			t.Fatalf("missing %s accepted", name)
		}
	}
}
