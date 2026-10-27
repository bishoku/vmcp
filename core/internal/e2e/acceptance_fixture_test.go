//go:build acceptance

package e2e

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bishoku/vmcp/core/internal/audit"
	"github.com/bishoku/vmcp/core/internal/mcp"
)

type acceptanceFixture struct {
	rest       bool
	tools      int
	resources  int
	sessions   sync.Map
	executions sync.Map
	duplicate  atomic.Int64
	count      atomic.Int64
}

func (f *acceptanceFixture) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if f.rest {
		f.serveREST(w, r)
		return
	}
	if r.Method == http.MethodDelete {
		f.sessions.Delete(r.Header.Get("Mcp-Session-Id"))
		w.WriteHeader(204)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", 405)
		return
	}
	if r.Header.Get("Authorization") != "Bearer synthetic-fixture" {
		http.Error(w, "unauthorized", 401)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "read failed", 400)
		return
	}
	msg, err := mcp.Decode(body)
	if err != nil {
		http.Error(w, "bad JSON RPC", 400)
		return
	}
	if msg.Method != mcp.MethodInitialize {
		route, ok := f.sessions.Load(r.Header.Get("Mcp-Session-Id"))
		if !ok || route != r.URL.Path {
			http.Error(w, "unknown fixture session", 404)
			return
		}
	}
	if msg.IsNotification() {
		w.WriteHeader(202)
		return
	}
	var result any
	switch msg.Method {
	case mcp.MethodInitialize:
		sid, _ := audit.NewID()
		f.sessions.Store(sid, r.URL.Path)
		w.Header().Set("Mcp-Session-Id", sid)
		result = mcp.InitializeResult{ProtocolVersion: mcp.SupportedVersions[0], Capabilities: mcp.ServerCapabilities{Tools: &mcp.ToolsCapability{}, Resources: &mcp.ResourcesCapability{}}, ServerInfo: mcp.Implementation{Name: "acceptance-synthetic", Version: "1"}}
	case mcp.MethodToolsList:
		var p mcp.ListToolsParams
		_ = json.Unmarshal(msg.Params, &p)
		start := 0
		if p.Cursor != "" {
			_, _ = fmt.Sscanf(p.Cursor, "%d", &start)
		}
		end := min(start+200, f.tools)
		items := make([]mcp.Tool, 0, end-start)
		for i := start; i < end; i++ {
			items = append(items, acceptanceTool(i))
		}
		page := mcp.ListToolsResult{Tools: items}
		if end < f.tools {
			page.NextCursor = fmt.Sprint(end)
		}
		result = page
	case "resources/list":
		items := make([]mcp.CatalogObject, 0, f.resources)
		for i := 0; i < f.resources; i++ {
			items = append(items, mcp.CatalogObject{"uri": json.RawMessage(fmt.Sprintf("%q", fmt.Sprintf("test://resource/%d", i))), "name": json.RawMessage(`"synthetic resource"`)})
		}
		result = map[string]any{"resources": items}
	case "resources/templates/list":
		result = map[string]any{"resourceTemplates": []any{}}
	case "prompts/list":
		result = map[string]any{"prompts": []any{}}
	case mcp.MethodToolsCall:
		var p mcp.CallToolParams
		_ = json.Unmarshal(msg.Params, &p)
		var args struct {
			CallID  string `json:"call_id"`
			Message string `json:"message"`
		}
		_ = json.Unmarshal(p.Arguments, &args)
		if args.CallID == "" {
			http.Error(w, "call_id required", 400)
			return
		}
		if _, loaded := f.executions.LoadOrStore(args.CallID, true); loaded {
			f.duplicate.Add(1)
		}
		f.count.Add(1)
		select {
		case <-time.After(5 * time.Millisecond):
		case <-r.Context().Done():
			return
		}
		text, _ := json.Marshal(map[string]string{"route": r.URL.Path, "tool": p.Name, "call_id": args.CallID, "message": args.Message})
		result = map[string]any{"content": []map[string]string{{"type": "text", "text": string(text)}}, "isError": false}
	default:
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(mcp.NewErrorResponse(msg.ID, mcp.CodeMethodNotFound, "fixture method unavailable"))
		return
	}
	response, _ := mcp.NewResult(msg.ID, result)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}
func acceptanceTool(i int) mcp.Tool {
	return mcp.Tool{Name: fmt.Sprintf("tool-%03d", i), Description: "Synthetic deterministic tool. Contact support@example.com. Returns route, name and caller nonce.", InputSchema: json.RawMessage(`{"type":"object","properties":{"call_id":{"type":"string"},"message":{"type":"string"}},"required":["call_id","message"],"additionalProperties":false}`), Annotations: json.RawMessage(`{"readOnlyHint":false,"idempotentHint":false}`)}
}

type acceptanceClient struct {
	url, session, token string
	http                *http.Client
	server              int
	rest                bool
}
type acceptanceResponse struct {
	result          json.RawMessage
	status          int
	requestID, node string
	err             error
}

func (c *acceptanceClient) request(ctx context.Context, method string, params any, id int64) acceptanceResponse {
	message, _ := mcp.NewRequest(mcp.NewIntID(id), method, params)
	if method == mcp.MethodInitialized {
		message.ID = mcp.ID{}
	}
	body, _ := json.Marshal(message)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, c.url, strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", "Bearer "+c.token)
	if c.session != "" {
		req.Header.Set("Mcp-Session-Id", c.session)
		req.Header.Set("MCP-Protocol-Version", mcp.SupportedVersions[0])
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return acceptanceResponse{err: err}
	}
	defer resp.Body.Close()
	out := acceptanceResponse{status: resp.StatusCode, requestID: resp.Header.Get("X-Request-Id"), node: resp.Header.Get("X-Acceptance-Node")}
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		out.err = fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
		return out
	}
	if method == mcp.MethodInitialize {
		c.session = resp.Header.Get("Mcp-Session-Id")
	}
	if resp.StatusCode == 202 {
		return out
	}
	if strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		scanner := bufio.NewScanner(io.LimitReader(resp.Body, 4<<20))
		scanner.Buffer(make([]byte, 4096), 4<<20)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			msg, err := mcp.Decode([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))))
			if err != nil {
				continue
			}
			if msg.ID.String() != message.ID.String() {
				continue
			}
			if msg.Error != nil {
				out.err = msg.Error
			} else {
				out.result = msg.Result
			}
			return out
		}
		out.err = scanner.Err()
		if out.err == nil {
			out.err = errors.New("SSE ended without matching response")
		}
		return out
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		out.err = err
		return out
	}
	msg, err := mcp.Decode(raw)
	if err != nil {
		out.err = err
		return out
	}
	if msg.ID.String() != message.ID.String() {
		out.err = errors.New("JSON RPC response ID mismatch")
		return out
	}
	if msg.Error != nil {
		out.err = msg.Error
	} else {
		out.result = msg.Result
	}
	return out
}
