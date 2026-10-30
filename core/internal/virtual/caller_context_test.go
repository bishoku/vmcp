package virtual

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/bishoku/vmcp/core/internal/callercontext"
	"github.com/bishoku/vmcp/core/internal/config"
	"github.com/bishoku/vmcp/core/internal/fixture"
	"github.com/bishoku/vmcp/core/internal/mcp"
	"github.com/bishoku/vmcp/core/internal/omni"
	"github.com/bishoku/vmcp/core/internal/restapi"
)

func TestCallerContextThroughVirtualMCP(t *testing.T) {
	for _, kind := range []string{"rest", "streamable_http", "sse"} {
		t.Run(kind, func(t *testing.T) {
			t.Setenv("VMCP_EGRESS_ALLOW_CIDRS", "127.0.0.0/8")
			var calls atomic.Int32
			physical := fixture.New(fixture.Options{})
			if kind == "sse" {
				physical = fixture.NewLegacySSE(fixture.Options{})
			}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if kind == "rest" {
					calls.Add(1)
					session := r.Header.Get("X-Chat-Session")
					token := r.Header.Get("Authorization")
					if token == "Bearer rejected" {
						http.Error(w, "invalid token", 401)
						return
					}
					if token != "Bearer token-"+session {
						t.Errorf("mixed context: session %q", session)
					}
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(map[string]string{"session": session})
					return
				}
				raw, _ := io.ReadAll(r.Body)
				r.Body = io.NopCloser(bytes.NewReader(raw))
				if msg, err := mcp.Decode(raw); err == nil && msg.Method == mcp.MethodToolsCall {
					calls.Add(1)
					var p mcp.CallToolParams
					_ = json.Unmarshal(msg.Params, &p)
					var meta map[string]map[string]string
					_ = json.Unmarshal(p.Meta, &meta)
					cc := meta["bank/context"]
					var args map[string]string
					_ = json.Unmarshal(p.Arguments, &args)
					if cc["token"] != "token-"+args["message"] || cc["session"] != args["message"] || len(cc) != 2 || len(meta) != 1 {
						t.Errorf("missing or mixed MCP context for %q", args["message"])
					}
				}
				physical.ServeHTTP(w, r)
			}))
			r := newRig(t)
			// Close upstream sessions before stopping the legacy SSE fixture.
			defer srv.Close()
			defer r.handler.Sessions.Close()
			cc := &callercontext.Config{Enabled: true, MetaKey: "bank/context", Fields: []callercontext.Field{{Name: "token", Required: true}, {Name: "session", Required: true}}}
			up := config.Upstream{ID: "u-context", Slug: "context", URL: srv.URL, Kind: "mcp", Transport: kind, CallerContext: cc}
			tool := mcp.Tool{Name: "context__echo", InputSchema: json.RawMessage(`{"type":"object"}`)}
			if kind == "rest" {
				cc.Fields[0].Header = "Authorization"
				cc.Fields[0].Bearer = true
				cc.Fields[1].Header = "X-Chat-Session"
				cfg, err := restapi.Compile(srv.URL, restapi.Config{Auth: restapi.Auth{Type: "none"}, AllowInsecureHTTP: true, TimeoutSeconds: 5, Operations: []restapi.Operation{{Name: "echo", Enabled: true, Method: "GET", Path: "/"}}}, nil)
				if err != nil {
					t.Fatal(err)
				}
				up.Kind = "rest"
				up.REST = cfg
			}
			snap := r.handler.Snap.(staticSnap).s
			snap.Upstreams[up.ID] = up
			snap.VirtualServers["support"] = config.VirtualServer{Slug: "support", Tools: []config.VirtualTool{{UpstreamID: up.ID, UpstreamName: "echo", Tool: tool}}}
			sid := r.initialize(t)
			v := snap.VirtualServers["support"]
			v.OmniEnabled = true
			snap.VirtualServers["support"] = v
			r.handler.OmniAllowAnonymous = true
			mux := http.NewServeMux()
			secret := strings.Repeat("s", 32)
			r.handler.RegisterOmni(mux, secret)
			dp := httptest.NewServer(mux)
			defer dp.Close()
			backend, _ := omni.NewBackend(dp.URL, secret)
			app := (&omni.Server{Backend: backend, Index: omni.NewIndex(nil), AllowAnonymous: true}).Handler()
			invoke := func(args, meta json.RawMessage) *mcp.Message {
				raw, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 99, "method": "tools/call", "params": map[string]any{"name": "invoke_tool", "arguments": map[string]any{"tool_ref": omni.Ref("support", tool.Name), "arguments": args}, "_meta": meta}})
				req := httptest.NewRequest("POST", "/mcp/v1/omni", bytes.NewReader(raw))
				req.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				app.ServeHTTP(w, req)
				var msg mcp.Message
				if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &msg) != nil {
					t.Errorf("omni: %d %s", w.Code, w.Body)
					return nil
				}
				return &msg
			}
			if out := invoke(json.RawMessage(`{}`), nil); out == nil || out.Error == nil || calls.Load() != 0 {
				t.Fatal("Omni required metadata not enforced")
			}
			var extra sync.WaitGroup
			for i := 0; i < 12; i++ {
				extra.Add(1)
				go func(i int) {
					defer extra.Done()
					value := fmt.Sprint(i)
					args, _ := json.Marshal(map[string]string{"message": value})
					if kind == "rest" {
						args = json.RawMessage(`{}`)
					}
					meta, _ := json.Marshal(map[string]any{"bank/context": map[string]string{"token": "token-" + value, "session": value, "ignored": "no"}, "unrelated": "no", "progressToken": "caller-progress"})
					out := invoke(args, meta)
					if out == nil || out.Error != nil || strings.Contains(string(out.Result), `"isError":true`) {
						t.Errorf("omni metadata call: %+v", out)
					}
				}(i)
			}
			extra.Wait()

			_, missing := r.rpc(t, "support", sid, mcp.MethodToolsCall, mcp.CallToolParams{Name: tool.Name, Arguments: json.RawMessage(`{}`)})
			if missing == nil || missing.Error == nil || calls.Load() != 12 {
				t.Fatal("required context did not prevent dispatch")
			}
			// Concurrent calls share one MCP session, but each must use its own refreshed token.
			var wg sync.WaitGroup
			for i := 0; i < 12; i++ {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					s := fmt.Sprint(i)
					meta, _ := json.Marshal(map[string]any{"bank/context": map[string]string{"token": "token-" + s, "session": s, "ignored": "not-forwarded"}, "unrelated": "not-forwarded"})
					args, _ := json.Marshal(map[string]string{"message": s})
					if kind == "rest" {
						args = json.RawMessage(`{}`)
					}
					_, out := r.rpc(t, "support", sid, mcp.MethodToolsCall, mcp.CallToolParams{Name: tool.Name, Arguments: args, Meta: meta})
					if out == nil || out.Error != nil || strings.Contains(string(out.Result), `"isError":true`) {
						t.Errorf("call failed: %+v", out)
					}
				}(i)
			}
			wg.Wait()
			if calls.Load() != 24 {
				t.Fatalf("calls=%d", calls.Load())
			}
			if kind == "rest" {
				_, out := r.rpc(t, "support", sid, mcp.MethodToolsCall, mcp.CallToolParams{Name: tool.Name, Arguments: json.RawMessage(`{}`), Meta: json.RawMessage(`{"bank/context":{"token":"rejected","session":"x"}}`)})
				if out == nil || !strings.Contains(string(out.Result), "401") || !strings.Contains(string(out.Result), `"isError":true`) {
					t.Fatalf("upstream rejection lost: %+v", out)
				}
			}
		})
	}
}
