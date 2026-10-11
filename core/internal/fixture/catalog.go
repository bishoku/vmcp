package fixture

import (
	"encoding/json"
	"fmt"
	"github.com/bishoku/vmcp/core/internal/mcp"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
)

// CatalogServer exercises resource/prompt semantics over real HTTP and SSE.
// It is an in-process interoperability fixture, not a production server.
type CatalogServer struct {
	mu      sync.Mutex
	streams map[string]chan string
	next    atomic.Int64
	Reads   atomic.Int64
}

func NewCatalog() *CatalogServer { return &CatalogServer{streams: map[string]chan string{}} }
func (c *CatalogServer) Update(uri string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, ch := range c.streams {
		select {
		case ch <- uri:
		default:
		}
	}
}
func (c *CatalogServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	sid := r.Header.Get("Mcp-Session-Id")
	if r.Method == http.MethodDelete {
		c.mu.Lock()
		delete(c.streams, sid)
		c.mu.Unlock()
		w.WriteHeader(204)
		return
	}
	if r.Method == http.MethodGet {
		c.mu.Lock()
		ch := c.streams[sid]
		c.mu.Unlock()
		if ch == nil {
			http.Error(w, "session missing", 404)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(": ready\n\n"))
		w.(http.Flusher).Flush()
		for {
			select {
			case <-r.Context().Done():
				return
			case uri := <-ch:
				msg, _ := mcp.NewRequest(mcp.ID{}, "notifications/resources/updated", map[string]string{"uri": uri})
				raw, _ := json.Marshal(msg)
				_, _ = fmt.Fprintf(w, "data: %s\n\n", raw)
				w.(http.Flusher).Flush()
			}
		}
	}
	raw, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	msg, err := mcp.Decode(raw)
	if err != nil {
		http.Error(w, "invalid", 400)
		return
	}
	if !msg.IsRequest() {
		w.WriteHeader(202)
		return
	}
	var p map[string]json.RawMessage
	_ = json.Unmarshal(msg.Params, &p)
	var result any
	switch msg.Method {
	case "initialize":
		sid = fmt.Sprintf("catalog-%d", c.next.Add(1))
		c.mu.Lock()
		c.streams[sid] = make(chan string, 16)
		c.mu.Unlock()
		w.Header().Set("Mcp-Session-Id", sid)
		result = mcp.InitializeResult{ProtocolVersion: mcp.ProtocolVersion20250618, ServerInfo: mcp.Implementation{Name: "catalog-fixture", Version: "1"}, Capabilities: mcp.ServerCapabilities{Resources: &mcp.ResourcesCapability{Subscribe: true, ListChanged: true}, Prompts: &mcp.ToolsCapability{ListChanged: true}, Completions: &struct{}{}}}
	case "resources/list":
		result = map[string]any{"resources": []any{map[string]any{"uri": "doc://public", "name": "Public", "title": "Public title", "description": "public description", "_meta": map[string]string{"fixture": "preserved"}}, map[string]string{"uri": "doc://hidden", "name": "Hidden"}, map[string]string{"uri": "doc://binary", "name": "Binary"}}}
	case "resources/templates/list":
		result = map[string]any{"resourceTemplates": []any{map[string]string{"uriTemplate": "doc://items/{id}", "name": "Items"}}}
	case "prompts/list":
		result = map[string]any{"prompts": []any{map[string]any{"name": "review", "title": "Review title", "description": "review description", "arguments": []any{map[string]any{"name": "subject", "required": true}}}}}
	case "resources/read":
		var uri string
		_ = json.Unmarshal(p["uri"], &uri)
		c.Reads.Add(1)
		if uri == "doc://binary" {
			result = map[string]any{"contents": []any{map[string]string{"uri": uri, "blob": "c2VjcmV0QGV4YW1wbGUuY29t"}}}
		} else {
			result = map[string]any{"contents": []any{map[string]string{"uri": uri, "text": "Contact person@example.com: " + uri}}}
		}
	case "prompts/get":
		result = map[string]any{"description": "Review", "messages": []any{map[string]any{"role": "user", "content": map[string]string{"type": "text", "text": "Review person@example.com"}}}}
	case "completion/complete":
		result = map[string]any{"completion": map[string]any{"values": []string{"alpha", "beta"}, "total": 2, "hasMore": false}}
	case "resources/subscribe", "resources/unsubscribe", "ping":
		result = map[string]any{}
	default:
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(mcp.NewErrorResponse(msg.ID, mcp.CodeMethodNotFound, "unsupported fixture method "+strings.TrimSpace(msg.Method)))
		return
	}
	response, _ := mcp.NewResult(msg.ID, result)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}
