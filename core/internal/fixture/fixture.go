// Package fixture is a small Streamable HTTP MCP server used for development
// and end-to-end tests. It is not part of the product.
package fixture

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/bishoku/vmcp/core/internal/mcp"
)

// Options configures the fixture server.
type Options struct {
	Token string // when set, requests must carry "Authorization: Bearer <Token>"
	SSE   bool   // answer tools/call as text/event-stream, preceded by a progress notification
}

// Tools lists the tools the fixture exposes.
func Tools() []mcp.Tool {
	obj := func(s string) json.RawMessage { return json.RawMessage(s) }
	return []mcp.Tool{
		{Name: "echo", Description: "Returns the given message", InputSchema: obj(`{"type":"object","properties":{"message":{"type":"string"}},"required":["message"]}`)},
		{Name: "add", Description: "Adds two numbers", InputSchema: obj(`{"type":"object","properties":{"a":{"type":"number"},"b":{"type":"number"}},"required":["a","b"]}`)},
		{Name: "whoami", Description: "Returns the Authorization header this server received", InputSchema: obj(`{"type":"object"}`)},
		{Name: "fail", Description: "Always returns a tool error", InputSchema: obj(`{"type":"object"}`)},
	}
}

type server struct {
	opts     Options
	mu       sync.Mutex
	sessions map[string]bool
}

// New returns the fixture HTTP handler.
func New(opts Options) http.Handler {
	return &server{opts: opts, sessions: map[string]bool{}}
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if s.opts.Token != "" && r.Header.Get("Authorization") != "Bearer "+s.opts.Token {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	switch r.Method {
	case http.MethodDelete:
		s.mu.Lock()
		delete(s.sessions, r.Header.Get("Mcp-Session-Id"))
		s.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	case http.MethodPost:
		s.post(w, r)
	default:
		w.Header().Set("Allow", "POST, DELETE")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *server) post(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "unreadable body", http.StatusBadRequest)
		return
	}
	m, err := mcp.Decode(body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if m.Method != mcp.MethodInitialize {
		s.mu.Lock()
		ok := s.sessions[r.Header.Get("Mcp-Session-Id")]
		s.mu.Unlock()
		if !ok {
			http.Error(w, "unknown session", http.StatusNotFound)
			return
		}
	}
	if !m.IsRequest() {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	var resp *mcp.Message
	switch m.Method {
	case mcp.MethodInitialize:
		var b [8]byte
		_, _ = rand.Read(b[:])
		sid := "fx-" + hex.EncodeToString(b[:])
		s.mu.Lock()
		s.sessions[sid] = true
		s.mu.Unlock()
		w.Header().Set("Mcp-Session-Id", sid)
		resp, _ = mcp.NewResult(m.ID, mcp.InitializeResult{
			ProtocolVersion: mcp.ProtocolVersion20250618,
			Capabilities:    mcp.ServerCapabilities{Tools: &mcp.ToolsCapability{}},
			ServerInfo:      mcp.Implementation{Name: "vmcp-fixture", Version: "0.1.0"},
		})
	case mcp.MethodPing:
		resp, _ = mcp.NewResult(m.ID, struct{}{})
	case mcp.MethodToolsList:
		resp, _ = mcp.NewResult(m.ID, mcp.ListToolsResult{Tools: Tools()})
	case mcp.MethodToolsCall:
		resp = s.call(m, r)
	default:
		resp = mcp.NewErrorResponse(m.ID, mcp.CodeMethodNotFound, "method not found: "+m.Method)
	}

	out, _ := json.Marshal(resp)
	if s.opts.SSE && m.Method == mcp.MethodToolsCall {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, `data: {"jsonrpc":"2.0","method":"notifications/progress","params":{"progress":1,"total":1}}`+"\n\n")
		_, _ = io.WriteString(w, "data: "+string(out)+"\n\n")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(out)
}

func (s *server) call(m *mcp.Message, r *http.Request) *mcp.Message {
	var p mcp.CallToolParams
	if err := json.Unmarshal(m.Params, &p); err != nil {
		return mcp.NewErrorResponse(m.ID, mcp.CodeInvalidParams, "invalid params")
	}
	text, isErr, rpcErr := run(p, r)
	if rpcErr != nil {
		return &mcp.Message{JSONRPC: mcp.JSONRPCVersion, ID: m.ID, Error: rpcErr}
	}
	res, _ := mcp.NewResult(m.ID, map[string]any{
		"content": []map[string]string{{"type": "text", "text": text}},
		"isError": isErr,
	})
	return res
}

func run(p mcp.CallToolParams, r *http.Request) (text string, isErr bool, rpcErr *mcp.Error) {
	switch p.Name {
	case "echo":
		var a struct {
			Message string `json:"message"`
		}
		if err := json.Unmarshal(p.Arguments, &a); err != nil {
			return "", false, &mcp.Error{Code: mcp.CodeInvalidParams, Message: "echo needs {message}"}
		}
		return a.Message, false, nil
	case "add":
		var a struct{ A, B float64 }
		if err := json.Unmarshal(p.Arguments, &a); err != nil {
			return "", false, &mcp.Error{Code: mcp.CodeInvalidParams, Message: "add needs {a, b}"}
		}
		return fmt.Sprintf("%g", a.A+a.B), false, nil
	case "whoami":
		return r.Header.Get("Authorization"), false, nil
	case "fail":
		return "intentional failure", true, nil
	default:
		return "", false, &mcp.Error{Code: mcp.CodeInvalidParams, Message: "unknown tool: " + p.Name}
	}
}
