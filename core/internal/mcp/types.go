package mcp

import "encoding/json"

// Protocol versions the gateway has been tested against.
const (
	ProtocolVersion20250326 = "2025-03-26"
	ProtocolVersion20250618 = "2025-06-18"
)

// SupportedVersions lists versions in order of preference (newest first).
var SupportedVersions = []string{ProtocolVersion20250618, ProtocolVersion20250326}

// NegotiateVersion returns the client's version if supported, otherwise the newest supported one.
func NegotiateVersion(requested string) string {
	for _, v := range SupportedVersions {
		if v == requested {
			return v
		}
	}
	return SupportedVersions[0]
}

type Implementation struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type InitializeParams struct {
	ProtocolVersion string          `json:"protocolVersion"`
	Capabilities    json.RawMessage `json:"capabilities"`
	ClientInfo      Implementation  `json:"clientInfo"`
}

type ToolsCapability struct {
	ListChanged bool `json:"listChanged"`
}

type ServerCapabilities struct {
	Tools *ToolsCapability `json:"tools,omitempty"`
}

type InitializeResult struct {
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    ServerCapabilities `json:"capabilities"`
	ServerInfo      Implementation     `json:"serverInfo"`
	Instructions    string             `json:"instructions,omitempty"`
}

// Tool describes a callable tool. InputSchema and Annotations are kept raw so
// schemas pass through the gateway untouched.
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"inputSchema"`
	Annotations json.RawMessage `json:"annotations,omitempty"`
}

type ListToolsParams struct {
	Cursor string `json:"cursor,omitempty"`
}

type ListToolsResult struct {
	Tools      []Tool `json:"tools"`
	NextCursor string `json:"nextCursor,omitempty"`
}

type CallToolParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

// Method names handled by the gateway.
const (
	MethodInitialize       = "initialize"
	MethodInitialized      = "notifications/initialized"
	MethodPing             = "ping"
	MethodToolsList        = "tools/list"
	MethodToolsCall        = "tools/call"
	MethodCancelled        = "notifications/cancelled"
	MethodProgress         = "notifications/progress"
	MethodToolsListChanged = "notifications/tools/list_changed"
)
