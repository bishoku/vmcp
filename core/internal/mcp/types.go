package mcp

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Protocol version identifiers. March is retained for compatibility tests,
// but is not advertised because its batching contract is not implemented.
const (
	ProtocolVersion20250326 = "2025-03-26"
	ProtocolVersion20250618 = "2025-06-18"
)

// SupportedVersions lists versions in order of preference (newest first).
var SupportedVersions = []string{ProtocolVersion20250618}

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

// SupportsVersion reports the protocol contract implemented by both transports.
func SupportsVersion(version string) bool {
	for _, v := range SupportedVersions {
		if v == version {
			return true
		}
	}
	return false
}

// ValidateInitializeParams checks required fields without rejecting extension capabilities.
func ValidateInitializeParams(raw json.RawMessage) (*InitializeParams, error) {
	var p InitializeParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, err
	}
	if err := validateHandshake(raw, "clientInfo"); err != nil {
		return nil, err
	}
	return &p, nil
}

// ValidateInitializeResult rejects incomplete or unsupported upstream handshakes.
func ValidateInitializeResult(raw json.RawMessage) (*InitializeResult, error) {
	var r InitializeResult
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, err
	}
	if err := validateHandshake(raw, "serverInfo"); err != nil {
		return nil, err
	}
	if !SupportsVersion(r.ProtocolVersion) {
		return nil, fmt.Errorf("unsupported upstream protocol version %q", r.ProtocolVersion)
	}
	return &r, nil
}

func validateHandshake(raw json.RawMessage, infoKey string) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return err
	}
	var version string
	if json.Unmarshal(fields["protocolVersion"], &version) != nil || version == "" {
		return fmt.Errorf("protocolVersion is required")
	}
	caps := bytes.TrimSpace(fields["capabilities"])
	if len(caps) == 0 || caps[0] != '{' {
		return fmt.Errorf("capabilities must be an object")
	}
	var info map[string]json.RawMessage
	if json.Unmarshal(fields[infoKey], &info) != nil || info == nil {
		return fmt.Errorf("%s must be an object", infoKey)
	}
	for _, key := range []string{"name", "version"} {
		var value string
		if json.Unmarshal(info[key], &value) != nil || string(info[key]) == "null" {
			return fmt.Errorf("%s.%s must be a string", infoKey, key)
		}
	}
	return nil
}
