// Package mcp implements the JSON-RPC 2.0 framing and the subset of MCP types the gateway needs.
package mcp

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

const JSONRPCVersion = "2.0"

// Standard JSON-RPC 2.0 error codes.
const (
	CodeParseError     = -32700
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeInternalError  = -32603
)

// ID is a JSON-RPC request id: a string, a number, or absent (notification).
// The raw JSON is kept verbatim so the id is echoed back unchanged.
type ID struct {
	raw json.RawMessage
}

// NewIntID returns a numeric id.
func NewIntID(n int64) ID { return ID{raw: json.RawMessage(fmt.Sprintf("%d", n))} }

// NewStringID returns a string id.
func NewStringID(s string) ID {
	b, _ := json.Marshal(s)
	return ID{raw: b}
}

// IsZero reports whether the id is absent.
func (i ID) IsZero() bool { return len(i.raw) == 0 || string(i.raw) == "null" }

func (i ID) String() string {
	if i.IsZero() {
		return ""
	}
	return string(i.raw)
}

func (i ID) MarshalJSON() ([]byte, error) {
	if len(i.raw) == 0 {
		return []byte("null"), nil
	}
	return i.raw, nil
}

func (i *ID) UnmarshalJSON(b []byte) error {
	t := bytes.TrimSpace(b)
	if len(t) == 0 || t[0] == '{' || t[0] == '[' || t[0] == 't' || t[0] == 'f' {
		return errors.New("id must be a string, number or null")
	}
	i.raw = append(i.raw[:0], t...)
	return nil
}

// Error is a JSON-RPC error object.
type Error struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *Error) Error() string { return fmt.Sprintf("jsonrpc error %d: %s", e.Code, e.Message) }

// Message is any JSON-RPC message: request, notification or response.
// A request has Method and ID; a notification has Method and no ID;
// a response has no Method and carries Result or Error.
type Message struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      ID              `json:"id,omitzero"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`
}

func (m *Message) IsRequest() bool      { return m.Method != "" && !m.ID.IsZero() }
func (m *Message) IsNotification() bool { return m.Method != "" && m.ID.IsZero() }
func (m *Message) IsResponse() bool     { return m.Method == "" && (m.Result != nil || m.Error != nil) }

// Decode parses one JSON-RPC message and validates its envelope.
// Batches are rejected: MCP 2025-06-18 removed JSON-RPC batching.
func Decode(data []byte) (*Message, error) {
	t := bytes.TrimSpace(data)
	if len(t) > 0 && t[0] == '[' {
		return nil, &Error{Code: CodeInvalidRequest, Message: "batch requests are not supported"}
	}
	var m Message
	if err := json.Unmarshal(t, &m); err != nil {
		return nil, &Error{Code: CodeParseError, Message: "parse error: " + err.Error()}
	}
	if m.JSONRPC != JSONRPCVersion {
		return nil, &Error{Code: CodeInvalidRequest, Message: `jsonrpc must be "2.0"`}
	}
	if !m.IsRequest() && !m.IsNotification() && !m.IsResponse() {
		return nil, &Error{Code: CodeInvalidRequest, Message: "not a valid request, notification or response"}
	}
	return &m, nil
}

// NewRequest builds a request message with marshaled params.
func NewRequest(id ID, method string, params any) (*Message, error) {
	m := &Message{JSONRPC: JSONRPCVersion, ID: id, Method: method}
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			return nil, err
		}
		m.Params = b
	}
	return m, nil
}

// NewResult builds a success response.
func NewResult(id ID, result any) (*Message, error) {
	b, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	return &Message{JSONRPC: JSONRPCVersion, ID: id, Result: b}, nil
}

// NewErrorResponse builds an error response.
func NewErrorResponse(id ID, code int, msg string) *Message {
	return &Message{JSONRPC: JSONRPCVersion, ID: id, Error: &Error{Code: code, Message: msg}}
}
