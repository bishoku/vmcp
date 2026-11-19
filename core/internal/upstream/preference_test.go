package upstream

import (
	"context"
	"encoding/json"
	"github.com/bishoku/vmcp/core/internal/mcp"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestDiscoveryNeverDowngradesOnServiceOrAuthenticationFailure(t *testing.T) {
	t.Setenv("VMCP_MCP_20260728", "1")
	t.Setenv("VMCP_EGRESS_ALLOW_CIDRS", "127.0.0.0/8")
	for _, status := range []int{401, 403, 500, 503} {
		var requests atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1); w.WriteHeader(status) }))
		c := NewClient(server.URL, nil, nil)
		_, err := c.Initialize(context.Background())
		server.Close()
		if err == nil || requests.Load() != 1 {
			t.Fatalf("HTTP %d: err=%v requests=%d", status, err, requests.Load())
		}
	}
}

func TestPreferenceExpiryRediscoversInPlaceUpgrade(t *testing.T) {
	t.Setenv("VMCP_MCP_20260728", "1")
	t.Setenv("VMCP_EGRESS_ALLOW_CIDRS", "127.0.0.0/8")
	var modern atomic.Bool
	var probes atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var msg mcp.Message
		_ = json.NewDecoder(r.Body).Decode(&msg)
		var result any
		switch msg.Method {
		case mcp.MethodDiscover:
			probes.Add(1)
			if !modern.Load() {
				http.Error(w, "legacy", 400)
				return
			}
			result = mcp.DiscoverServer("fixture", "", mcp.ServerCapabilities{})
		case mcp.MethodInitialize:
			result = mcp.InitializeResult{ProtocolVersion: mcp.ProtocolVersion20250618, ServerInfo: mcp.Implementation{Name: "fixture", Version: "1"}}
		case mcp.MethodInitialized:
			w.WriteHeader(202)
			return
		default:
			t.Error(msg.Method)
			return
		}
		response, _ := mcp.NewResult(msg.ID, result)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()
	now := time.Now()
	preferences := PreferenceCache{Clock: func() time.Time { return now }}
	key := preferenceKey{url: server.URL}
	prepare := func(wantModern bool) {
		c := NewClient(server.URL, nil, nil)
		if err := preferences.PrepareInvocation(context.Background(), c, key.url, key.generation); err != nil || c.isModern() != wantModern {
			t.Fatalf("modern=%v err=%v", c.isModern(), err)
		}
	}
	prepare(false)
	prepare(false)
	modern.Store(true)
	now = now.Add(6 * time.Minute)
	prepare(true)
	prepare(true)
	if probes.Load() != 2 {
		t.Fatalf("discovery probes %d", probes.Load())
	}
}
