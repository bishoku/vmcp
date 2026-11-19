package virtual

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/bishoku/vmcp/core/internal/store"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bishoku/vmcp/core/internal/callercontext"
	"github.com/bishoku/vmcp/core/internal/config"
	"github.com/bishoku/vmcp/core/internal/fixture"
	"github.com/bishoku/vmcp/core/internal/mcp"
	"github.com/bishoku/vmcp/core/internal/restapi"
)

func TestRequestHeadersThroughVirtualMCP(t *testing.T) {
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
					if r.Header.Get("X-Upstream-Token") != "forward-"+r.Header.Get("X-Chat-Session") {
						t.Error("REST header isolation failed")
					}
					if r.Header.Get("X-Unselected") != "" {
						t.Error("unselected header forwarded")
					}
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
					if r.Header.Get("X-Upstream-Token") != "forward-"+cc["session"] {
						t.Error("MCP header isolation failed")
					}
					if r.Header.Get("X-Unselected") != "" {
						t.Error("unselected header forwarded")
					}
					var args map[string]string
					_ = json.Unmarshal(p.Arguments, &args)
					if cc["token"] != "token-"+args["message"] || cc["session"] != args["message"] || len(cc) != 2 || len(meta) != 1 {
						t.Errorf("missing or mixed MCP context for %q", args["message"])
					}
				} else if r.Header.Get("X-Upstream-Token") != "" {
					t.Error("header leaked to non-tool request")
				}
				physical.ServeHTTP(w, r)
			}))
			r := newRig(t)
			// Close upstream sessions before stopping the legacy SSE fixture.
			defer srv.Close()
			defer r.handler.Sessions.Close()
			cc := &callercontext.Config{Enabled: true, MetaKey: "bank/context", Fields: []callercontext.Field{{Name: "token", Required: true}, {Name: "session", Required: true}}}
			up := config.Upstream{ID: "u-context", Slug: "context", URL: srv.URL, Kind: "mcp", Transport: kind, CallerContext: cc, RequestHeaderMappings: []callercontext.HeaderMapping{{Source: "X-Agent-Token", Target: "X-Upstream-Token", Required: true}}}
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
			_, missing := r.rpc(t, "support", sid, mcp.MethodToolsCall, mcp.CallToolParams{Name: tool.Name, Arguments: json.RawMessage(`{}`)})
			if missing == nil || missing.Error == nil || calls.Load() != 0 {
				t.Fatal("required context did not prevent dispatch")
			}
			_, missingHeader := r.rpc(t, "support", sid, mcp.MethodToolsCall, mcp.CallToolParams{Name: tool.Name, Arguments: json.RawMessage(`{}`), Meta: json.RawMessage(`{"bank/context":{"token":"token-x","session":"x"}}`)})
			if missingHeader == nil || missingHeader.Error == nil || calls.Load() != 0 {
				t.Fatal("required header did not prevent dispatch")
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
					_, out := r.rpcHeaders(t, "support", sid, mcp.MethodToolsCall, mcp.CallToolParams{Name: tool.Name, Arguments: args, Meta: meta}, http.Header{"X-Agent-Token": {"forward-" + s}, "X-Unselected": {"private"}})
					if out == nil || out.Error != nil || strings.Contains(string(out.Result), `"isError":true`) {
						t.Errorf("call failed: %+v", out)
					}
				}(i)
			}
			wg.Wait()
			if calls.Load() != 12 {
				t.Fatalf("calls=%d", calls.Load())
			}
			if kind == "rest" {
				_, out := r.rpcHeaders(t, "support", sid, mcp.MethodToolsCall, mcp.CallToolParams{Name: tool.Name, Arguments: json.RawMessage(`{}`), Meta: json.RawMessage(`{"bank/context":{"token":"rejected","session":"x"}}`)}, http.Header{"X-Agent-Token": {"forward-x"}})
				if out == nil || !strings.Contains(string(out.Result), "401") || !strings.Contains(string(out.Result), `"isError":true`) {
					t.Fatalf("upstream rejection lost: %+v", out)
				}
			}
		})
	}
}

func (r *rig) rpcHeaders(t *testing.T, slug, sid, method string, params any, headers http.Header) (*http.Response, *mcp.Message) {
	t.Helper()
	req, _ := mcp.NewRequest(mcp.NewIntID(1), method, params)
	b, _ := json.Marshal(req)
	hr, _ := http.NewRequest(http.MethodPost, r.gw.URL+"/mcp/v1/virtual/"+slug, bytes.NewReader(b))
	hr.Header.Set("Content-Type", "application/json")
	hr.Header.Set("Accept", "application/json, text/event-stream")
	if sid != "" {
		hr.Header.Set("Mcp-Session-Id", sid)
	}
	for name, values := range headers {
		hr.Header[name] = values
	}
	resp, err := http.DefaultClient.Do(hr)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	m, err := mcp.Decode(data)
	if err != nil {
		return resp, nil
	}
	return resp, m
}

func TestModernCallsIsolatePrincipalsAndHeaders(t *testing.T) {
	t.Setenv("VMCP_MCP_20260728", "1")
	t.Setenv("VMCP_EGRESS_ALLOW_CIDRS", "127.0.0.0/8")
	r := newRig(t)
	h := r.handler
	recorder := &omniAuditCapture{}
	h.Audit = recorder
	snap := h.Snap.Snapshot()
	v := snap.VirtualServers["support"]
	v.AuthRequired = true
	snap.VirtualServers["support"] = v
	snap.AgentKeys = map[string]config.AgentKey{}
	for _, id := range []string{"one", "two"} {
		snap.AgentKeys[string(store.HashKey(id))] = config.AgentKey{ID: id, Slug: id, Servers: map[string]bool{"support": true}}
	}
	physical := fixture.New(fixture.Options{})
	sessions := sync.Map{}
	var closed atomic.Int64
	var calls atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "DELETE" {
			closed.Add(1)
		}
		if r.Header.Get("Mcp-Session-Id") != "" && r.Method == "POST" {
			raw, _ := io.ReadAll(r.Body)
			r.Body = io.NopCloser(bytes.NewReader(raw))
			m, err := mcp.Decode(raw)
			if err == nil && m.Method == mcp.MethodToolsCall {
				calls.Add(1)
				var p struct{ Arguments struct{ Message string } }
				_ = json.Unmarshal(m.Params, &p)
				if r.Header.Get("X-Forward") != p.Arguments.Message {
					t.Error("dynamic header crossed principals")
				}
				if _, loaded := sessions.LoadOrStore(r.Header.Get("Mcp-Session-Id"), p.Arguments.Message); loaded {
					t.Error("reused an invocation session")
				}
			}
		}
		physical.ServeHTTP(w, r)
	}))
	defer srv.Close()
	up := snap.Upstreams["u-pg"]
	up.URL = srv.URL
	up.RequestHeaderMappings = []callercontext.HeaderMapping{{Source: "X-Source", Target: "X-Forward", Required: true}}
	snap.Upstreams["u-pg"] = up
	v.Tools[1].UpstreamName = "echo"
	mux := http.NewServeMux()
	h.Register(mux)
	gateway := httptest.NewServer(mux)
	defer gateway.Close()
	local := &rig{gw: gateway}
	var wg sync.WaitGroup
	for _, id := range []string{"one", "two"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			meta := map[string]any{"io.modelcontextprotocol/protocolVersion": "2026-07-28", "io.modelcontextprotocol/clientCapabilities": map[string]any{}}
			_, m := local.rpcHeaders(t, "support", "", "tools/call", map[string]any{"name": "tables", "arguments": map[string]string{"message": id}, "_meta": meta}, http.Header{"Mcp-Protocol-Version": {"2026-07-28"}, "Mcp-Method": {"tools/call"}, "Mcp-Name": {"tables"}, "Authorization": {"Bearer " + id}, "X-Source": {id}})
			if m == nil || m.Error != nil || !strings.Contains(string(m.Result), id) {
				t.Errorf("failed %s: %+v", id, m)
			}
		}()
	}
	wg.Wait()
	deadline := time.Now().Add(time.Second)
	for closed.Load() != 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if calls.Load() != 2 || closed.Load() != 2 {
		t.Fatalf("calls=%d closed=%d", calls.Load(), closed.Load())
	}
	actors := map[string]bool{}
	for _, e := range recorder.events {
		if e.Action == "tools/call" {
			actors[e.ActorID] = true
		}
	}
	if !actors["one"] || !actors["two"] {
		t.Fatalf("audit actors %v", actors)
	}
}
