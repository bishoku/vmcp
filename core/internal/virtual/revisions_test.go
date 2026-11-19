package virtual

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bishoku/vmcp/core/internal/config"
	"github.com/bishoku/vmcp/core/internal/mcp"
	"github.com/bishoku/vmcp/core/internal/omni"
	"github.com/bishoku/vmcp/core/internal/store"
)

func TestPinnedRevisionMatrix(t *testing.T) {
	if os.Getenv("VMCP_REQUIRE_SDK_INTEROP") != "1" {
		t.Skip("requires pinned web SDKs")
	}
	t.Setenv("VMCP_MCP_20260728", "1")
	t.Setenv("VMCP_EGRESS_ALLOW_CIDRS", "127.0.0.0/8")
	for _, endpoint := range []string{"virtual", "omni"} {
		for _, in := range []string{"legacy", "modern"} {
			for _, out := range []string{"legacy", "modern"} {
				t.Run(endpoint+"/"+in+"/"+out, func(t *testing.T) {
					rig := newRig(t)
					defer rig.handler.Sessions.Close()
					var requests, calls atomic.Int64
					fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						requests.Add(1)
						if r.Method == "DELETE" {
							w.WriteHeader(204)
							return
						}
						var msg mcp.Message
						if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
							t.Error(err)
							return
						}
						if out == "legacy" && r.Header.Get("MCP-Protocol-Version") == mcp.ProtocolVersion20260728 {
							http.Error(w, "unsupported version", 400)
							return
						}
						var result any
						if out == "modern" {
							if r.Header.Get("Mcp-Method") != msg.Method || r.Header.Get("MCP-Protocol-Version") != mcp.ProtocolVersion20260728 || r.Header.Get("Mcp-Session-Id") != "" {
								t.Errorf("unexpected modern headers on %s", msg.Method)
								http.Error(w, "headers", 400)
								return
							}
						}
						switch msg.Method {
						case mcp.MethodDiscover:
							result = mcp.DiscoverServer("fixture", "", mcp.ServerCapabilities{Tools: &mcp.ToolsCapability{}})
						case mcp.MethodInitialize:
							if out != "legacy" {
								t.Error("modern handshake")
							}
							w.Header().Set("Mcp-Session-Id", "fixture-session")
							result = mcp.InitializeResult{ProtocolVersion: mcp.ProtocolVersion20250618, ServerInfo: mcp.Implementation{Name: "fixture", Version: "1"}, Capabilities: mcp.ServerCapabilities{Tools: &mcp.ToolsCapability{}}}
						case mcp.MethodInitialized:
							w.WriteHeader(202)
							return
						case mcp.MethodToolsCall:
							var p mcp.CallToolParams
							_ = json.Unmarshal(msg.Params, &p)
							if p.Name != "list_tables" || (out == "modern" && r.Header.Get("Mcp-Name") != p.Name) || (out == "legacy" && r.Header.Get("Mcp-Session-Id") != "fixture-session") {
								t.Error("incorrect upstream call")
							}
							calls.Add(1)
							result = map[string]any{"content": []map[string]string{{"type": "text", "text": p.Name}}}
						default:
							t.Errorf("unexpected %s", msg.Method)
							http.Error(w, "unexpected", 400)
							return
						}
						reply, _ := mcp.NewResult(msg.ID, result)
						if out == "modern" {
							mcp.CompleteResult(msg.Method, reply)
						}
						w.Header().Set("Content-Type", "application/json")
						_ = json.NewEncoder(w).Encode(reply)
					}))
					defer fixture.Close()
					h := rig.handler
					snap := h.Snap.Snapshot()
					up := snap.Upstreams["u-pg"]
					up.URL = fixture.URL
					snap.Upstreams["u-pg"] = up
					v := snap.VirtualServers["support"]
					v.OmniEnabled = true
					snap.VirtualServers["support"] = v
					snap.AgentKeys = map[string]config.AgentKey{string(store.HashKey("runtime-key")): {ID: "matrix", Slug: "agent", Servers: map[string]bool{"support": true}}}
					url := rig.gw.URL + "/mcp/v1/virtual/support"
					if endpoint == "omni" {
						secret := strings.Repeat("s", 32)
						mux := http.NewServeMux()
						h.RegisterOmni(mux, secret)
						dp := httptest.NewServer(mux)
						defer dp.Close()
						backend, _ := omni.NewBackend(dp.URL, secret)
						index := omni.NewIndex(nil)
						index.Refresh(context.Background(), backend)
						server := httptest.NewServer((&omni.Server{Backend: backend, Index: index}).Handler())
						defer server.Close()
						url = server.URL + "/mcp/v1/omni"
					}
					ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
					defer cancel()
					output, err := exec.CommandContext(ctx, "node", "../../../web/scripts/mcp-revision-matrix.mjs", url, in, endpoint).CombinedOutput()
					if err != nil {
						t.Fatalf("%v: %s", err, output)
					}
					if calls.Load() != 21 {
						t.Fatalf("upstream calls %d", calls.Load())
					}
					t.Logf("%s upstream HTTP requests=%d", output, requests.Load())
				})
			}
		}
	}
}
