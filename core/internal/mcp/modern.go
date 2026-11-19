package mcp

import (
	"encoding/json"
	"net/http"
	"os"
)

const ProtocolVersion20260728 = "2026-07-28"
const MethodDiscover = "server/discover"

// ModernEnabled gates protocol rollout across data-plane, control-plane and Omni replicas.
func ModernEnabled() bool { return os.Getenv("VMCP_MCP_20260728") == "1" }

type RequestMetadata struct {
	Version      string                     `json:"io.modelcontextprotocol/protocolVersion"`
	Client       Implementation             `json:"io.modelcontextprotocol/clientInfo"`
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

func ValidateModern(h http.Header, msg *Message) *Message {
	var p struct {
		Name string
		Meta RequestMetadata `json:"_meta"`
	}
	if !msg.IsRequest() || json.Unmarshal(msg.Params, &p) != nil {
		return NewErrorResponse(msg.ID, CodeInvalidParams, "modern request parameters required")
	}
	if len(h.Values("MCP-Protocol-Version")) != 1 || p.Meta.Version != ProtocolVersion20260728 || len(h.Values("Mcp-Method")) != 1 || h.Get("Mcp-Method") != msg.Method {
		return NewErrorResponse(msg.ID, -32020, "protocol version or method header mismatch")
	}
	if msg.Method == MethodToolsCall && (p.Name == "" || len(h.Values("Mcp-Name")) != 1 || h.Get("Mcp-Name") != p.Name) {
		return NewErrorResponse(msg.ID, -32020, "tool name header mismatch")
	}
	if p.Meta.Client.Name == "" || p.Meta.Client.Version == "" || h.Get("Mcp-Session-Id") != "" {
		return NewErrorResponse(msg.ID, CodeInvalidRequest, "invalid modern client metadata or session header")
	}
	return nil
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
