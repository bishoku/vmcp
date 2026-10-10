package upstream

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/bishoku/vmcp/core/internal/mcp"
)

// fixture is a minimal in-memory Streamable HTTP MCP server for tests.
type fixture struct {
	sse        bool // answer requests as text/event-stream
	token      string
	mu         sync.Mutex
	session    string
	deleted    bool
	sawVersion string
}

func (f *fixture) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if f.token != "" && r.Header.Get("Authorization") != "Bearer "+f.token {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if r.Method == http.MethodDelete {
			f.mu.Lock()
			f.deleted = true
			f.mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
			return
		}
		body, _ := io.ReadAll(r.Body)
		m, err := mcp.Decode(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if m.Method != mcp.MethodInitialize {
			f.mu.Lock()
			ok := r.Header.Get("Mcp-Session-Id") == f.session && f.session != ""
			f.sawVersion = r.Header.Get("MCP-Protocol-Version")
			f.mu.Unlock()
			if !ok {
				http.Error(w, "no session", http.StatusNotFound)
				return
			}
		}
		if m.IsNotification() {
			w.WriteHeader(http.StatusAccepted)
			return
		}

		var resp *mcp.Message
		switch m.Method {
		case mcp.MethodInitialize:
			f.mu.Lock()
			f.session = "sess-1"
			f.mu.Unlock()
			w.Header().Set("Mcp-Session-Id", "sess-1")
			resp, _ = mcp.NewResult(m.ID, mcp.InitializeResult{
				ProtocolVersion: mcp.ProtocolVersion20250618,
				Capabilities:    mcp.ServerCapabilities{Tools: &mcp.ToolsCapability{}},
				ServerInfo:      mcp.Implementation{Name: "fixture", Version: "1"},
			})
		case mcp.MethodToolsList:
			var p mcp.ListToolsParams
			_ = json.Unmarshal(m.Params, &p)
			schema := json.RawMessage(`{"type":"object"}`)
			if p.Cursor == "" {
				resp, _ = mcp.NewResult(m.ID, mcp.ListToolsResult{
					Tools:      []mcp.Tool{{Name: "echo", InputSchema: schema}},
					NextCursor: "p2",
				})
			} else {
				resp, _ = mcp.NewResult(m.ID, mcp.ListToolsResult{
					Tools: []mcp.Tool{{Name: "add", InputSchema: schema}},
				})
			}
		case mcp.MethodToolsCall:
			var p mcp.CallToolParams
			_ = json.Unmarshal(m.Params, &p)
			if p.Name != "echo" {
				resp = mcp.NewErrorResponse(m.ID, mcp.CodeInvalidParams, "unknown tool")
				break
			}
			resp, _ = mcp.NewResult(m.ID, map[string]any{
				"content": []map[string]string{{"type": "text", "text": string(p.Arguments)}},
			})
		default:
			resp = mcp.NewErrorResponse(m.ID, mcp.CodeMethodNotFound, "no such method")
		}

		out, _ := json.Marshal(resp)
		if f.sse {
			w.Header().Set("Content-Type", "text/event-stream")
			// A server notification precedes the actual response.
			_, _ = io.WriteString(w, "data: {\"jsonrpc\":\"2.0\",\"method\":\"notifications/progress\",\"params\":{}}\n\n")
			_, _ = io.WriteString(w, "data: "+string(out)+"\n\n")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(out)
	})
}

func newTestClient(t *testing.T, f *fixture) *Client {
	t.Helper()
	srv := httptest.NewServer(f.handler())
	t.Cleanup(srv.Close)
	headers := map[string]string{}
	if f.token != "" {
		headers["Authorization"] = "Bearer " + f.token
	}
	return NewClient(srv.URL, headers, nil)
}

func TestClientLifecycle(t *testing.T) {
	for _, sse := range []bool{false, true} {
		name := "json"
		if sse {
			name = "sse"
		}
		t.Run(name, func(t *testing.T) {
			f := &fixture{sse: sse, token: "secret"}
			c := newTestClient(t, f)
			ctx := context.Background()

			var serverMsgs int
			c.OnServerMessage = func(*mcp.Message) { serverMsgs++ }

			res, err := c.Initialize(ctx)
			if err != nil {
				t.Fatalf("initialize: %v", err)
			}
			if res.ServerInfo.Name != "fixture" || c.SessionID() != "sess-1" {
				t.Fatalf("bad handshake: %+v session=%q", res, c.SessionID())
			}

			tools, err := c.ListTools(ctx)
			if err != nil {
				t.Fatalf("list: %v", err)
			}
			if len(tools) != 2 || tools[0].Name != "echo" || tools[1].Name != "add" {
				t.Fatalf("pagination not followed: %+v", tools)
			}
			if f.sawVersion != mcp.ProtocolVersion20250618 {
				t.Fatalf("protocol version header = %q", f.sawVersion)
			}

			out, err := c.CallTool(ctx, "echo", json.RawMessage(`{"x":1}`))
			if err != nil {
				t.Fatalf("call: %v", err)
			}
			if !strings.Contains(string(out), `{\"x\":1}`) {
				t.Fatalf("unexpected result: %s", out)
			}

			if sse && serverMsgs == 0 {
				t.Fatal("server notifications on the SSE stream were dropped")
			}

			if err := c.Close(ctx); err != nil {
				t.Fatalf("close: %v", err)
			}
			if !f.deleted || c.SessionID() != "" {
				t.Fatal("session was not terminated")
			}
		})
	}
}

func TestClientRPCErrorSurfaced(t *testing.T) {
	c := newTestClient(t, &fixture{})
	ctx := context.Background()
	if _, err := c.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	_, err := c.CallTool(ctx, "nope", nil)
	var rpcErr *mcp.Error
	if !errors.As(err, &rpcErr) || rpcErr.Code != mcp.CodeInvalidParams {
		t.Fatalf("want *mcp.Error invalid params, got %v", err)
	}
}

func TestClientSessionExpired(t *testing.T) {
	f := &fixture{}
	c := newTestClient(t, f)
	ctx := context.Background()
	if _, err := c.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	f.session = "different"
	f.mu.Unlock()

	if _, err := c.ListTools(ctx); !errors.Is(err, ErrSessionExpired) {
		t.Fatalf("want ErrSessionExpired, got %v", err)
	}
}

func TestClientUnauthorized(t *testing.T) {
	f := &fixture{token: "secret"}
	srv := httptest.NewServer(f.handler())
	t.Cleanup(srv.Close)
	c := NewClient(srv.URL, nil, nil)
	_, err := c.Initialize(context.Background())
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("want 401 error, got %v", err)
	}
}

func TestClientContextCancel(t *testing.T) {
	c := newTestClient(t, &fixture{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Initialize(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
}
