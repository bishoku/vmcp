package virtual

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bishoku/vmcp/core/internal/config"
	"github.com/bishoku/vmcp/core/internal/fixture"
	"github.com/bishoku/vmcp/core/internal/mcp"
	"github.com/bishoku/vmcp/core/internal/omni"
	"github.com/bishoku/vmcp/core/internal/store"
	"github.com/bishoku/vmcp/core/internal/upstream"
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
					checkRevisionPair(t, endpoint, in, out)
				})
			}
		}
	}
}

func checkRevisionPair(t *testing.T, endpoint, in, out string) {

	rig := newRig(t)
	defer rig.handler.Sessions.Close()
	var requests, calls, opened, peak, arrived atomic.Int64
	var active sync.Map
	var activeCount atomic.Int64
	barrier := make(chan struct{})
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Method == "DELETE" {
			if _, ok := active.LoadAndDelete(r.Header.Get("Mcp-Session-Id")); !ok {
				t.Error("closing unknown upstream session")
			} else {
				activeCount.Add(-1)
			}
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
			sid := fmt.Sprintf("fixture-%d", opened.Add(1))
			active.Store(sid, true)
			n := activeCount.Add(1)
			for prev := peak.Load(); prev < n && !peak.CompareAndSwap(prev, n); prev = peak.Load() {
			}
			w.Header().Set("Mcp-Session-Id", sid)
			result = mcp.InitializeResult{ProtocolVersion: mcp.ProtocolVersion20250618, ServerInfo: mcp.Implementation{Name: "fixture", Version: "1"}, Capabilities: mcp.ServerCapabilities{Tools: &mcp.ToolsCapability{}}}
		case mcp.MethodInitialized:
			w.WriteHeader(202)
			return
		case mcp.MethodToolsCall:
			var p mcp.CallToolParams
			_ = json.Unmarshal(msg.Params, &p)
			_, live := active.Load(r.Header.Get("Mcp-Session-Id"))
			if p.Name != "list_tables" || (out == "modern" && r.Header.Get("Mcp-Name") != p.Name) || (out == "legacy" && !live) {
				t.Error("incorrect upstream call")
			}
			if strings.Contains(string(p.Arguments), "burst") {
				if arrived.Add(1) == 4 {
					close(barrier)
				}
				select {
				case <-barrier:
				case <-time.After(5 * time.Second):
					t.Error("parallel calls stalled")
				}
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
	mux := http.NewServeMux()
	h.Register(mux)
	gateway := httptest.NewServer(mux)
	defer gateway.Close()
	url := gateway.URL + "/mcp/v1/virtual/support"
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
	if calls.Load() != 25 {
		t.Fatalf("upstream calls %d", calls.Load())
	}
	wantPeak := int64(0)
	if out == "legacy" {
		wantPeak = 4
		if endpoint == "virtual" && in == "legacy" {
			wantPeak = 1
		}
	}
	if peak.Load() != wantPeak {
		t.Fatalf("peak sessions %d, want %d", peak.Load(), wantPeak)
	}
	if endpoint == "omni" || in == "modern" {
		if h.Sessions.Len() != 0 {
			t.Fatal("registered a transient call as a long-lived session")
		}
		awaitEnd := time.Now().Add(time.Second)
		for activeCount.Load() != 0 && time.Now().Before(awaitEnd) {
			time.Sleep(time.Millisecond)
		}
		if activeCount.Load() != 0 {
			t.Fatal("upstream sessions leaked")
		}
	}
	t.Logf("%s upstream HTTP requests=%d peak sessions=%d", output, requests.Load(), peak.Load())
}

func TestModernPrimaryLegacyFallback(t *testing.T) {
	t.Setenv("VMCP_MCP_20260728", "1")
	t.Setenv("VMCP_EGRESS_ALLOW_CIDRS", "127.0.0.0/8")
	var available atomic.Bool
	available.Store(true)
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !available.Load() {
			http.Error(w, "unavailable", 503)
			return
		}
		var m mcp.Message
		_ = json.NewDecoder(r.Body).Decode(&m)
		result := any(mcp.DiscoverServer("primary", "", mcp.ServerCapabilities{Tools: &mcp.ToolsCapability{}}))
		switch m.Method {
		case mcp.MethodDiscover:
		case mcp.MethodToolsCall:
			result = map[string]any{"resultType": "complete", "content": []map[string]string{{"type": "text", "text": "primary"}}}
		default:
			t.Errorf("unexpected %s", m.Method)
		}
		reply, _ := mcp.NewResult(m.ID, result)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(reply)
	}))
	defer primary.Close()
	legacy := httptest.NewServer(fixture.New(fixture.Options{}))
	defer legacy.Close()
	up := config.Upstream{URL: primary.URL, FallbackURLs: []string{legacy.URL}}
	preferences := new(upstream.PreferenceCache)
	call := func(up config.Upstream, want string) {
		c, err := dial(context.Background(), up, preferences, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close(context.Background())
		result, err := c.CallTool(context.Background(), "echo", json.RawMessage(`{"message":"fallback"}`))
		if err != nil || !strings.Contains(string(result), want) {
			t.Fatalf("result %s, err %v", result, err)
		}
	}
	call(up, "primary")
	// A warm dispatch may already have executed. Fail it once, then let a
	// separate invocation discover the failed primary and use the fallback.
	available.Store(false)
	c, err := dial(context.Background(), up, preferences, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = c.CallTool(context.Background(), "echo", json.RawMessage(`{}`)); err == nil {
		t.Fatal("expected an ambiguous failure")
	}
	_ = c.Close(context.Background())
	call(up, "fallback")
}

func TestModernCancellationClosesLegacySession(t *testing.T) {
	t.Setenv("VMCP_MCP_20260728", "1")
	t.Setenv("VMCP_EGRESS_ALLOW_CIDRS", "127.0.0.0/8")
	started := make(chan struct{})
	closed := make(chan struct{})
	legacy := fixture.New(fixture.Options{})
	physical := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "DELETE" {
			legacy.ServeHTTP(w, r)
			close(closed)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(raw))
		if msg, err := mcp.Decode(raw); err == nil && msg.Method == mcp.MethodToolsCall {
			close(started)
			select {
			case <-r.Context().Done():
			case <-time.After(5 * time.Second):
				t.Error("lost upstream cancellation")
			}
			return
		}
		legacy.ServeHTTP(w, r)
	}))
	defer physical.Close()
	rig := newRig(t)
	snap := rig.handler.Snap.Snapshot()
	up := snap.Upstreams["u-pg"]
	up.URL = physical.URL
	snap.Upstreams["u-pg"] = up
	mux := http.NewServeMux()
	rig.handler.Register(mux)
	gw := httptest.NewServer(mux)
	defer gw.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "POST", gw.URL+"/mcp/v1/virtual/support", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"tables","arguments":{},"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}}}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json,text/event-stream")
	req.Header.Set("Mcp-Method", "tools/call")
	req.Header.Set("Mcp-Name", "tables")
	req.Header.Set("Mcp-Protocol-Version", "2026-07-28")
	done := make(chan error, 1)
	go func() {
		resp, err := http.DefaultClient.Do(req)
		if resp != nil {
			resp.Body.Close()
		}
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("dispatch timed out")
	}
	cancel()
	if err := <-done; err == nil {
		t.Fatal("expected cancellation")
	}
	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("legacy cleanup timed out")
	}
}
