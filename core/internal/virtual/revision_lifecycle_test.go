package virtual

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bishoku/vmcp/core/internal/config"
	"github.com/bishoku/vmcp/core/internal/mcp"
	"github.com/bishoku/vmcp/core/internal/upstream"
)

// Both managers resolve the same logical client session through a shared store.
// Each opens its own physical connection and advances its own discovery clock.
func TestActiveRevisionAcrossIndependentManagers(t *testing.T) {
	t.Setenv("VMCP_MCP_20260728", "1")
	t.Setenv("VMCP_EGRESS_ALLOW_CIDRS", "127.0.0.0/8")
	var modern atomic.Bool
	var probes, handshakes, modernCalls, legacyCalls, loggingCalls atomic.Int32
	deleted := make(chan string, 4)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "DELETE" {
			deleted <- r.Header.Get("Mcp-Session-Id")
			w.WriteHeader(204)
			return
		}
		var msg mcp.Message
		if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
			t.Error(err)
			return
		}
		var result any
		switch msg.Method {
		case mcp.MethodDiscover:
			probes.Add(1)
			if !modern.Load() {
				http.Error(w, "legacy", 400)
				return
			}
			result = mcp.DiscoverServer("fixture", "", mcp.ServerCapabilities{Tools: &mcp.ToolsCapability{}, Logging: &struct{}{}})
		case mcp.MethodInitialize:
			w.Header().Set("Mcp-Session-Id", fmt.Sprintf("legacy-%d", handshakes.Add(1)))
			result = mcp.InitializeResult{ProtocolVersion: mcp.ProtocolVersion20250618, ServerInfo: mcp.Implementation{Name: "legacy", Version: "1"}}
		case mcp.MethodInitialized:
			w.WriteHeader(202)
			return
		case "logging/setLevel":
			loggingCalls.Add(1)
			result = struct{}{}
		case mcp.MethodToolsCall:
			if r.Header.Get("Mcp-Session-Id") != "" {
				legacyCalls.Add(1)
			} else if r.Header.Get("MCP-Protocol-Version") == mcp.ProtocolVersion20260728 {
				modernCalls.Add(1)
			} else {
				t.Error("missing wire revision")
			}
			result = map[string]any{"content": []any{}}
		default:
			t.Errorf("unexpected %s", msg.Method)
			return
		}
		reply, _ := mcp.NewResult(msg.ID, result)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(reply)
	}))
	defer srv.Close()
	a, b := NewManager(), NewManager()
	defer a.Close()
	defer b.Close()
	nowA, nowB := time.Now(), time.Now()
	a.preferences.Clock = func() time.Time { return nowA }
	b.preferences.Clock = func() time.Time { return nowB }
	a.Store = newMemorySessions()
	b.Store = a.Store
	ctx := context.Background()
	sa, err := a.Create("support", mcp.ProtocolVersion20250618)
	if err != nil {
		t.Fatal(err)
	}
	if err = a.Persist(ctx, sa); err != nil {
		t.Fatal(err)
	}
	sb, err := b.Resolve(ctx, sa.id)
	if err != nil || sb == nil {
		t.Fatalf("resolve: %v", err)
	}
	up := config.Upstream{ID: "u", URL: srv.URL}
	invoke := func(s *Session) {
		t.Helper()
		c, release, err := s.client(ctx, up)
		if err != nil {
			t.Fatal(err)
		}
		defer release()
		if err := upstream.SetLoggingLevel(ctx, c, "warning"); err != nil {
			t.Fatal(err)
		}
		if _, err := c.CallTool(ctx, "echo", json.RawMessage(`{}`)); err != nil {
			t.Fatal(err)
		}
	}
	invoke(sa)
	invoke(sb)
	if probes.Load() != 2 || handshakes.Load() != 2 {
		t.Fatal("replicas shared discovery or upstream sessions")
	}
	nowA = nowA.Add(6 * time.Minute)
	invoke(sa)
	if probes.Load() != 3 || handshakes.Load() != 2 {
		t.Fatal("unchanged revision replaced the active legacy session")
	}
	// Keep a lease on A's original session while a later caller upgrades it.
	_, releaseOld, err := sa.client(ctx, up)
	if err != nil {
		t.Fatal(err)
	}
	modern.Store(true)
	nowA = nowA.Add(6 * time.Minute)
	invoke(sa)
	invoke(sb)
	if legacyCalls.Load() != 4 || modernCalls.Load() != 1 || probes.Load() != 4 {
		t.Fatal("replicas failed to select independently")
	}
	select {
	case sid := <-deleted:
		t.Fatalf("closed leased session %s", sid)
	default:
	}
	releaseOld()
	select {
	case sid := <-deleted:
		if sid != "legacy-1" {
			t.Fatal(sid)
		}
	case <-time.After(time.Second):
		t.Fatal("old session not retired")
	}
	nowB = nowB.Add(6 * time.Minute)
	invoke(sb)
	fresh, err := a.newSession("support", mcp.ProtocolVersion20260728)
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.closeUpstreamsSync()
	invoke(fresh)
	// A new manager starts cold against an upstream that already advertises
	// modern logging. Both its first and its next invocation must skip setup.
	cold := NewManager()
	defer cold.Close()
	for range 2 {
		one, err := cold.newSession("support", mcp.ProtocolVersion20260728)
		if err != nil {
			t.Fatal(err)
		}
		invoke(one)
		one.closeUpstreamsSync()
	}
	if modernCalls.Load() != 5 || probes.Load() != 6 || handshakes.Load() != 2 || loggingCalls.Load() != 0 {
		t.Fatalf("modern calls=%d probes=%d handshakes=%d logging=%d", modernCalls.Load(), probes.Load(), handshakes.Load(), loggingCalls.Load())
	}
}

func TestExpiredUpstreamConsumesInvocationRetryBudget(t *testing.T) {
	t.Setenv("VMCP_EGRESS_ALLOW_CIDRS", "127.0.0.0/8")
	for _, tc := range []struct {
		name, rule string
		want       int32
	}{{"default", "", 1}, {"once", "once", 2}} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "DELETE" {
					w.WriteHeader(204)
					return
				}
				var msg mcp.Message
				_ = json.NewDecoder(r.Body).Decode(&msg)
				switch msg.Method {
				case mcp.MethodInitialize:
					w.Header().Set("Mcp-Session-Id", "fixture")
					reply, _ := mcp.NewResult(msg.ID, mcp.InitializeResult{ProtocolVersion: mcp.ProtocolVersion20250618, ServerInfo: mcp.Implementation{Name: "fixture", Version: "1"}})
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(reply)
				case mcp.MethodInitialized:
					w.WriteHeader(202)
				case mcp.MethodToolsCall:
					calls.Add(1)
					http.Error(w, "expired", 404)
				default:
					t.Error("unexpected request")
				}
			}))
			defer srv.Close()
			rig := newRig(t)
			defer rig.handler.Sessions.Close()
			sess, err := rig.handler.Sessions.Create("support", mcp.ProtocolVersion20250618)
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.WithValue(context.Background(), retryInvocationKey{}, tc.rule)
			_, err = rig.handler.callResilient(ctx, sess, config.Upstream{ID: "u", URL: srv.URL}, config.VirtualTool{UpstreamName: "echo"}, json.RawMessage(`{}`))
			if err == nil || calls.Load() != tc.want {
				t.Fatalf("calls=%d err=%v", calls.Load(), err)
			}
		})
	}
}
