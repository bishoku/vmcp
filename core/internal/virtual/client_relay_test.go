package virtual

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"github.com/bishoku/vmcp/core/internal/config"
	"github.com/bishoku/vmcp/core/internal/delivery"
	"github.com/bishoku/vmcp/core/internal/mcp"
	"github.com/bishoku/vmcp/core/internal/secrets"
	"github.com/bishoku/vmcp/core/internal/store"
	"github.com/bishoku/vmcp/core/internal/testdb"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNegotiatedClientRequestsAndDeclinesAcrossNodes(t *testing.T) {
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
	var mu sync.Mutex
	pending := map[string]chan *mcp.Message{}
	received := make(chan *mcp.Message, 3)
	capabilities := make(chan json.RawMessage, 1)
	rootChanges := make(chan struct{}, 1)
	physical := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "DELETE" {
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
			mu.Lock()
			ch := pending[msg.ID.String()]
			mu.Unlock()
			if ch != nil {
				ch <- msg
			}
			w.WriteHeader(202)
			return
		}
		if msg.IsNotification() {
			if msg.Method == "notifications/roots/list_changed" {
				select {
				case rootChanges <- struct{}{}:
				default:
				}
			}
			w.WriteHeader(202)
			return
		}
		if msg.Method == "initialize" {
			var p mcp.InitializeParams
			_ = json.Unmarshal(msg.Params, &p)
			select {
			case capabilities <- p.Capabilities:
			default:
			}
			response, _ := mcp.NewResult(msg.ID, mcp.InitializeResult{ProtocolVersion: mcp.ProtocolVersion20250618, ServerInfo: mcp.Implementation{Name: "features", Version: "1"}, Capabilities: mcp.ServerCapabilities{Tools: &mcp.ToolsCapability{}, Logging: &struct{}{}}})
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(response)
			return
		}
		if msg.Method == "logging/setLevel" {
			response, _ := mcp.NewResult(msg.ID, struct{}{})
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(response)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.(http.Flusher).Flush()
		requests := []struct {
			method string
			params any
		}{{"sampling/createMessage", map[string]any{"maxTokens": 64, "messages": []any{map[string]any{"role": "user", "content": map[string]string{"type": "text", "text": "summarize"}}}}}, {"roots/list", nil}, {"elicitation/create", map[string]any{"message": "Approve sharing?", "requestedSchema": map[string]any{"type": "object", "properties": map[string]any{"name": map[string]string{"type": "string"}}, "required": []string{"name"}}}}}
		for index, request := range requests {
			id := mcp.NewIntID(int64(100 + index))
			ch := make(chan *mcp.Message, 1)
			mu.Lock()
			pending[id.String()] = ch
			mu.Unlock()
			serverRequest, _ := mcp.NewRequest(id, request.method, request.params)
			b, _ := json.Marshal(serverRequest)
			_, _ = w.Write([]byte("data: " + string(b) + "\n\n"))
			w.(http.Flusher).Flush()
			select {
			case response := <-ch:
				received <- response
			case <-r.Context().Done():
				return
			case <-time.After(5 * time.Second):
				return
			}
		}
		log, _ := mcp.NewRequest(mcp.ID{}, "notifications/message", map[string]any{"level": "warning", "data": "Authorization: Bearer fixture-secret"})
		b, _ := json.Marshal(log)
		_, _ = w.Write([]byte("data: " + string(b) + "\n\n"))
		result, _ := mcp.NewResult(msg.ID, map[string]any{"content": []any{map[string]string{"type": "text", "text": "finished"}}, "structuredContent": map[string]bool{"ok": true}, "_meta": map[string]string{"kept": "yes"}})
		b, _ = json.Marshal(result)
		_, _ = w.Write([]byte("data: " + string(b) + "\n\n"))
		w.(http.Flusher).Flush()
	}))
	defer physical.Close()
	rig := newRig(t)
	snap := rig.handler.Snap.Snapshot()
	up := snap.Upstreams["u-gh"]
	up.URL = physical.URL
	snap.Upstreams["u-gh"] = up
	vs := snap.VirtualServers["support"]
	vs.ClientFeatures = config.ClientFeatures{Sampling: true, Roots: true, Elicitation: true}
	snap.VirtualServers["support"] = vs
	rig.handler.Delivery = broker
	rig.handler.Sessions.Store = PGSessionStore{Pool: pool}
	defer rig.handler.StopWorkers()
	second := &Handler{Snap: rig.handler.Snap, Sessions: NewManager(), Delivery: broker, Log: rig.handler.Log}
	second.Sessions.Store = PGSessionStore{Pool: pool}
	mux := http.NewServeMux()
	second.Register(mux)
	nodeB := httptest.NewServer(mux)
	defer nodeB.Close()
	defer second.StopWorkers()
	var sid string
	post := func(base string, message *mcp.Message) *http.Response {
		t.Helper()
		raw, _ := json.Marshal(message)
		r, _ := http.NewRequest("POST", base+"/mcp/v1/virtual/support", bytes.NewReader(raw))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Accept", "application/json, text/event-stream")
		if sid != "" {
			r.Header.Set("Mcp-Session-Id", sid)
			r.Header.Set("MCP-Protocol-Version", mcp.ProtocolVersion20250618)
		}
		response, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		return response
	}
	init, _ := mcp.NewRequest(mcp.NewIntID(1), "initialize", mcp.InitializeParams{ProtocolVersion: mcp.ProtocolVersion20250618, Capabilities: json.RawMessage(`{"sampling":{},"roots":{"listChanged":true},"elicitation":{}}`), ClientInfo: mcp.Implementation{Name: "host", Version: "1"}})
	response := post(rig.gw.URL, init)
	sid = response.Header.Get("Mcp-Session-Id")
	response.Body.Close()
	initialized, _ := mcp.NewRequest(mcp.ID{}, mcp.MethodInitialized, nil)
	response = post(rig.gw.URL, initialized)
	response.Body.Close()
	request, _ := mcp.NewRequest(mcp.NewIntID(2), mcp.MethodToolsCall, mcp.CallToolParams{Name: "github__create_issue"})
	response = post(rig.gw.URL, request)
	defer response.Body.Close()
	scanner := bufio.NewScanner(response.Body)
	seen := map[string]bool{}
	finished := false
	sanitized := false
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "fixture-secret") {
			t.Fatal("credential-bearing log reached client")
		}
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		message, err := mcp.Decode([]byte(strings.TrimPrefix(line, "data: ")))
		if err != nil {
			t.Fatal(err)
		}
		if message.IsRequest() {
			if message.ID.String() == "100" || message.ID.String() == "101" || message.ID.String() == "102" {
				t.Fatal("physical ID exposed")
			}
			seen[message.Method] = true
			var result any
			switch message.Method {
			case "sampling/createMessage":
				changed, _ := mcp.NewRequest(mcp.ID{}, "notifications/roots/list_changed", nil)
				accepted := post(nodeB.URL, changed)
				accepted.Body.Close()
				if accepted.StatusCode != 202 {
					t.Fatal("negotiated root notification rejected")
				}
				select {
				case <-rootChanges:
				case <-time.After(3 * time.Second):
					t.Fatal("cross-node root notification did not reach active physical session")
				}
				result = map[string]any{"role": "assistant", "model": "host-model", "content": map[string]string{"type": "text", "text": "host approved"}}
			case "roots/list":
				result = map[string]any{"roots": []any{map[string]string{"uri": "file:///host/workspace", "name": "Host"}}}
			case "elicitation/create":
				result = map[string]string{"action": "decline"}
			default:
				t.Fatalf("unexpected client request %s", message.Method)
			}
			reply, _ := mcp.NewResult(message.ID, result)
			accepted := post(nodeB.URL, reply)
			accepted.Body.Close()
			if accepted.StatusCode != 202 {
				t.Fatalf("cross-node client reply %d", accepted.StatusCode)
			}
		} else if message.Method == "notifications/message" {
			sanitized = strings.Contains(line, "omitted")
		} else if message.IsResponse() {
			if message.Error != nil || !strings.Contains(string(message.Result), "structuredContent") || !strings.Contains(string(message.Result), "kept") {
				t.Fatalf("tool result metadata lost: %s", line)
			}
			finished = true
			break
		}
	}
	if !finished || len(seen) != 3 || !sanitized {
		t.Fatalf("incomplete relay: finished=%v methods=%v sanitized=%v", finished, seen, sanitized)
	}
	offered := <-capabilities
	if !strings.Contains(string(offered), "sampling") || !strings.Contains(string(offered), "elicitation") {
		t.Fatal("physical handshake did not negotiate client features")
	}
	for i := 0; i < 3; i++ {
		reply := <-received
		if reply.ID.String() != []string{"100", "101", "102"}[i] {
			t.Fatal("server response ID was not restored")
		}
		if i == 2 && !strings.Contains(string(reply.Result), "decline") {
			t.Fatal("client refusal was bypassed")
		}
	}
}
