package virtual

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/bishoku/vmcp/core/internal/admission"
	"github.com/bishoku/vmcp/core/internal/delivery"
	"github.com/bishoku/vmcp/core/internal/mcp"
	"github.com/bishoku/vmcp/core/internal/secrets"
	"github.com/bishoku/vmcp/core/internal/store"
	"github.com/bishoku/vmcp/core/internal/testdb"
)

func TestAdmissionDisconnectedHTTPRetainsWorkerSlot(t *testing.T) {
	ctx := context.Background()
	url := testdb.New(t)
	if err := store.Migrate(ctx, url); err != nil {
		t.Fatal(err)
	}
	pool, err := store.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	box, _ := secrets.NewBox(base64.StdEncoding.EncodeToString(make([]byte, 32)))
	started, finish := make(chan struct{}), make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(finish) })
	physical := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		msg, err := mcp.Decode(raw)
		if err != nil {
			w.WriteHeader(400)
			return
		}
		if msg.IsNotification() {
			w.WriteHeader(202)
			return
		}
		var result *mcp.Message
		if msg.Method == mcp.MethodInitialize {
			result, _ = mcp.NewResult(msg.ID, mcp.InitializeResult{ProtocolVersion: mcp.ProtocolVersion20250618, ServerInfo: mcp.Implementation{Name: "fixture", Version: "1"}})
		} else {
			close(started)
			select {
			case <-finish:
			case <-r.Context().Done():
				return
			}
			result, _ = mcp.NewResult(msg.ID, map[string]any{"content": []any{map[string]any{"type": "text", "text": "done"}}})
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	}))
	defer physical.Close()
	rig := newRig(t)
	snap := rig.handler.Snap.Snapshot()
	up := snap.Upstreams["u-gh"]
	up.URL = physical.URL
	snap.Upstreams["u-gh"] = up
	snap.Admission = admission.Settings{Total: 1, PerServer: 1, PerPrincipal: 1}
	rig.handler.Delivery = &delivery.Store{Pool: pool, Box: box}
	rig.handler.Sessions.Store = PGSessionStore{Pool: pool}
	defer rig.handler.StopWorkers()
	defer once.Do(func() { close(finish) })
	sid := rig.initialize(t)
	request, _ := mcp.NewRequest(mcp.NewIntID(91), mcp.MethodToolsCall, mcp.CallToolParams{Name: "github__create_issue", Arguments: json.RawMessage(`{}`)})
	raw, _ := json.Marshal(request)
	reqCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	req, _ := http.NewRequestWithContext(reqCtx, "POST", rig.gw.URL+"/mcp/v1/virtual/support", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Mcp-Session-Id", sid)
	done := make(chan struct{})
	go func() {
		defer close(done)
		resp, _ := http.DefaultClient.Do(req)
		if resp != nil {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not start")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("HTTP did not disconnect")
	}
	// Probe through the handler after disconnect: a running tool still fills the pod.
	resp, _ := rig.rpc(t, "support", sid, mcp.MethodToolsList, nil)
	if resp.StatusCode != 429 {
		t.Fatal("disconnected worker released capacity", resp.StatusCode)
	}
	once.Do(func() { close(finish) })
	deadline := time.Now().Add(5 * time.Second)
	for {
		rig.handler.budgetMu.Lock()
		count := len(rig.handler.budgets)
		rig.handler.budgetMu.Unlock()
		if count == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("completed worker leaked slot")
		}
		time.Sleep(10 * time.Millisecond)
	}
	resp, _ = rig.rpc(t, "support", sid, mcp.MethodToolsList, nil)
	if resp.StatusCode != 200 {
		t.Fatal("capacity not restored", resp.StatusCode)
	}
}
