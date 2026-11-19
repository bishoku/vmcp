package mcp

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"os"
	"strings"
)

const ProtocolVersion20260728 = "2026-07-28"
const MethodDiscover = "server/discover"

// ModernEnabled gates protocol rollout across data-plane, control-plane and Omni replicas.
func ModernEnabled() bool { return os.Getenv("VMCP_MCP_20260728") == "1" }

type RequestMetadata struct {
	Version      string                     `json:"io.modelcontextprotocol/protocolVersion"`
	Client       *Implementation            `json:"io.modelcontextprotocol/clientInfo"`
	Capabilities map[string]json.RawMessage `json:"io.modelcontextprotocol/clientCapabilities"`
}

type Discovery struct {
	ResultType        string             `json:"resultType"`
	SupportedVersions []string           `json:"supportedVersions"`
	Capabilities      ServerCapabilities `json:"capabilities"`
	Meta              struct {
		Server Implementation `json:"io.modelcontextprotocol/serverInfo"`
	} `json:"_meta"`
	Instructions string `json:"instructions,omitempty"`
}

func DiscoverServer(name, instructions string, caps ServerCapabilities) Discovery {
	d := Discovery{ResultType: "complete", SupportedVersions: []string{ProtocolVersion20260728, ProtocolVersion20250618}, Capabilities: caps, Instructions: instructions}
	d.Meta.Server = Implementation{Name: name, Version: "0.1.0"}
	return d
}

// ModernRequest classifies body metadata even if the HTTP mirror is missing.
func ModernRequest(h http.Header, msg *Message) bool {
	if !ModernEnabled() {
		return false
	}
	if h.Get("MCP-Protocol-Version") == ProtocolVersion20260728 {
		return true
	}
	var p struct {
		Meta RequestMetadata `json:"_meta"`
	}
	return json.Unmarshal(msg.Params, &p) == nil && p.Meta.Version == ProtocolVersion20260728
}

// ValidateModern consumes the per-hop envelope before passing parameters to local handlers.
func ValidateModern(h http.Header, msg *Message) *Message {
	var p struct {
		Name string
		URI  string
		Meta RequestMetadata `json:"_meta"`
	}
	if !msg.IsRequest() || json.Unmarshal(msg.Params, &p) != nil {
		return NewErrorResponse(msg.ID, CodeInvalidParams, "modern request parameters required")
	}
	if len(h.Values("MCP-Protocol-Version")) != 1 || h.Get("MCP-Protocol-Version") != ProtocolVersion20260728 || p.Meta.Version != ProtocolVersion20260728 || len(h.Values("Mcp-Method")) != 1 || h.Get("Mcp-Method") != msg.Method {
		return NewErrorResponse(msg.ID, -32020, "protocol version or method header mismatch")
	}
	var name string
	switch msg.Method {
	case MethodToolsCall, "prompts/get":
		name = p.Name
	case "resources/read":
		name = p.URI
	}
	if (msg.Method == MethodToolsCall || msg.Method == "resources/read" || msg.Method == "prompts/get") && (name == "" || len(h.Values("Mcp-Name")) != 1 || h.Get("Mcp-Name") != RoutingValue(name)) {
		return NewErrorResponse(msg.ID, -32020, "tool name header mismatch")
	}
	if p.Meta.Capabilities == nil || (p.Meta.Client != nil && (p.Meta.Client.Name == "" || p.Meta.Client.Version == "")) || h.Get("Mcp-Session-Id") != "" {
		return NewErrorResponse(msg.ID, CodeInvalidRequest, "invalid modern client metadata or session header")
	}
	var params map[string]json.RawMessage
	var meta map[string]json.RawMessage
	_ = json.Unmarshal(msg.Params, &params)
	_ = json.Unmarshal(params["_meta"], &meta)
	for _, key := range []string{"protocolVersion", "clientInfo", "clientCapabilities"} {
		delete(meta, "io.modelcontextprotocol/"+key)
	}
	if len(meta) == 0 {
		delete(params, "_meta")
	} else {
		params["_meta"], _ = json.Marshal(meta)
	}
	msg.Params, _ = json.Marshal(params)
	return nil
}

// RoutingValue encodes values that would be changed or rejected by HTTP field parsing.
func RoutingValue(value string) string {
	encode := value == "" || strings.TrimSpace(value) != value || (strings.HasPrefix(value, "=?base64?") && strings.HasSuffix(value, "?="))
	for _, r := range value {
		if r != '\t' && (r < 32 || r > 126) {
			encode = true
			break
		}
	}
	if encode {
		return "=?base64?" + base64.StdEncoding.EncodeToString([]byte(value)) + "?="
	}
	return value
}

func CompleteResult(method string, msg *Message) {
	if msg.Error != nil {
		return
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(msg.Result, &fields) == nil && fields != nil {
		switch method {
		case MethodDiscover, MethodToolsList, "resources/list", "resources/read", "resources/templates/list":
			fields["ttlMs"] = json.RawMessage("0")
			fields["cacheScope"] = json.RawMessage(`"private"`)
		}
		fields["resultType"] = json.RawMessage(`"complete"`)
		msg.Result, _ = json.Marshal(fields)
	}
}
