package virtual

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bishoku/vmcp/core/internal/delivery"
	"github.com/bishoku/vmcp/core/internal/mcp"
	"github.com/bishoku/vmcp/core/internal/secrets"
	"github.com/bishoku/vmcp/core/internal/store"
	"github.com/bishoku/vmcp/core/internal/testdb"
)

func TestDuplexReplayAndRepliesAcrossNodes(t *testing.T) {
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
	broker := &delivery.Store{Pool: pool, Box: box}
	var calls atomic.Int32
	pingReply := make(chan struct{}, 1)
	cancellations := make(chan mcp.ID, 1)
	physical := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(204)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		msg, err := mcp.Decode(raw)
		if err != nil {
			http.Error(w, "invalid", 400)
			return
		}
		if msg.IsResponse() {
			select {
			case pingReply <- struct{}{}:
			default:
			}
			w.WriteHeader(202)
			return
		}
		if msg.IsNotification() {
			if msg.Method == mcp.MethodCancelled {
				var params struct {
					RequestID mcp.ID `json:"requestId"`
				}
				json.Unmarshal(msg.Params, &params)
				select {
				case cancellations <- params.RequestID:
				default:
				}
			}
			w.WriteHeader(202)
			return
		}
		if msg.Method == mcp.MethodInitialize {
			result, _ := mcp.NewResult(msg.ID, mcp.InitializeResult{ProtocolVersion: mcp.ProtocolVersion20250618, ServerInfo: mcp.Implementation{Name: "test", Version: "1"}, Capabilities: mcp.ServerCapabilities{}})
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(result)
			return
		}
		calls.Add(1)
		var params mcp.CallToolParams
		json.Unmarshal(msg.Params, &params)
		var meta map[string]any
		json.Unmarshal(params.Meta, &meta)
		progress, _ := mcp.NewRequest(mcp.ID{}, mcp.MethodProgress, map[string]any{"progressToken": meta["progressToken"], "progress": 1})
		ping, _ := mcp.NewRequest(mcp.NewStringID("physical-ping"), mcp.MethodPing, nil)
		w.Header().Set("Content-Type", "text/event-stream")
		if strings.Contains(string(params.Arguments), "cancel") {
			b, _ := json.Marshal(progress)
			w.Write([]byte("data: " + string(b) + "\n\n"))
			w.(http.Flusher).Flush()
			<-r.Context().Done()
			return
		}
		for _, message := range []*mcp.Message{progress, ping} {
			b, _ := json.Marshal(message)
			w.Write([]byte("data: " + string(b) + "\n\n"))
			w.(http.Flusher).Flush()
		}
		select {
		case <-pingReply:
		case <-r.Context().Done():
			return
		case <-time.After(5 * time.Second):
			return
		}
		result, _ := mcp.NewResult(msg.ID, map[string]any{"content": []any{map[string]any{"type": "text", "text": "finished"}}})
		b, _ := json.Marshal(result)
		w.Write([]byte("data: " + string(b) + "\n\n"))
		w.(http.Flusher).Flush()
	}))
	defer physical.Close()
	rig := newRig(t)
	snap := rig.handler.Snap.Snapshot()
	up := snap.Upstreams["u-gh"]
	up.URL = physical.URL
	snap.Upstreams["u-gh"] = up
	rig.handler.Delivery = broker
	rig.handler.Sessions.Store = PGSessionStore{Pool: pool}
	second := &Handler{Snap: rig.handler.Snap, Sessions: NewManager(), Delivery: broker, Log: rig.handler.Log}
	second.Sessions.Store = PGSessionStore{Pool: pool}
	mux := http.NewServeMux()
	second.Register(mux)
	nodeB := httptest.NewServer(mux)
	defer nodeB.Close()
	defer rig.handler.StopWorkers()
	defer second.StopWorkers()
	sid := rig.initialize(t)
	post := func(base string, message *mcp.Message) *http.Response {
		b, _ := json.Marshal(message)
		req, _ := http.NewRequest("POST", base+"/mcp/v1/virtual/support", bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		req.Header.Set("Mcp-Session-Id", sid)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}
	request, _ := mcp.NewRequest(mcp.NewIntID(9), mcp.MethodToolsCall, mcp.CallToolParams{Name: "github__create_issue", Meta: json.RawMessage(`{"progressToken":"client-progress"}`)})
	response := post(rig.gw.URL, request)
	scanner := bufio.NewScanner(response.Body)
	lastID := ""
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "id: ") {
			lastID = strings.TrimPrefix(line, "id: ")
		}
		if strings.HasPrefix(line, "data: ") {
			if !strings.Contains(line, "client-progress") {
				t.Fatal("progress token not translated")
			}
			break
		}
	}
	if lastID == "" {
		t.Fatal("missing replay cursor")
	}
	response.Body.Close()
	// Resume on another node. Wait for the old owner's HTTP cleanup to release its lease.
	var resumed *http.Response
	for deadline := time.Now().Add(6 * time.Second); time.Now().Before(deadline); time.Sleep(25 * time.Millisecond) {
		req, _ := http.NewRequest("GET", nodeB.URL+"/mcp/v1/virtual/support", nil)
		req.Header.Set("Accept", "text/event-stream")
		req.Header.Set("Mcp-Session-Id", sid)
		req.Header.Set("Last-Event-ID", lastID)
		resumed, err = http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		if resumed.StatusCode == 200 {
			break
		}
		resumed.Body.Close()
	}
	if resumed == nil || resumed.StatusCode != 200 {
		t.Fatal("cross-node resume failed")
	}
	defer resumed.Body.Close()
	scan := bufio.NewScanner(resumed.Body)
	var serverRequest *mcp.Message
	for scan.Scan() {
		line := scan.Text()
		if strings.HasPrefix(line, "data: ") {
			serverRequest, err = mcp.Decode([]byte(strings.TrimPrefix(line, "data: ")))
			if err == nil && serverRequest.IsRequest() {
				break
			}
		}
	}
	if serverRequest == nil || serverRequest.Method != mcp.MethodPing {
		t.Fatal("server request lost across reconnect")
	}
	reply, _ := mcp.NewResult(serverRequest.ID, struct{}{})
	accepted := post(nodeB.URL, reply)
	accepted.Body.Close()
	if accepted.StatusCode != 202 {
		t.Fatalf("reply status %d", accepted.StatusCode)
	}
	found := false
	for scan.Scan() {
		if strings.Contains(scan.Text(), "finished") {
			found = true
			break
		}
	}
	if !found || calls.Load() != 1 {
		t.Fatalf("lost or duplicated tool: final=%v calls=%d", found, calls.Load())
	}
	cancelCall, _ := mcp.NewRequest(mcp.NewIntID(10), mcp.MethodToolsCall, mcp.CallToolParams{Name: "github__create_issue", Arguments: json.RawMessage(`{"cancel":true}`), Meta: json.RawMessage(`{"progressToken":"cancel-progress"}`)})
	cancelResponse := post(rig.gw.URL, cancelCall)
	cancelScan := bufio.NewScanner(cancelResponse.Body)
	for cancelScan.Scan() {
		if strings.Contains(cancelScan.Text(), "cancel-progress") {
			break
		}
	}
	notification, _ := mcp.NewRequest(mcp.ID{}, mcp.MethodCancelled, map[string]any{"requestId": mcp.NewIntID(10)})
	acceptedCancel := post(nodeB.URL, notification)
	acceptedCancel.Body.Close()
	if acceptedCancel.StatusCode != 202 {
		t.Fatal("cross-node cancellation rejected")
	}
	select {
	case upstreamID := <-cancellations:
		if upstreamID.String() == "10" || upstreamID.IsZero() {
			t.Fatal("cancellation request ID was not translated")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("cancellation did not reach upstream")
	}
	cancelResponse.Body.Close()
	wait, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	if err := rig.handler.WaitWorkers(wait); err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest("GET", nodeB.URL+"/mcp/v1/virtual/support", nil)
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Mcp-Session-Id", sid)
	stream, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Body.Close()
	revokedAt := time.Now()
	if err := rig.handler.Sessions.End(ctx, sid); err != nil {
		t.Fatal(err)
	}
	_, _ = io.ReadAll(stream.Body)
	if time.Since(revokedAt) > 3*time.Second {
		t.Fatal("revocation did not close stream within authorization check interval")
	}

}
