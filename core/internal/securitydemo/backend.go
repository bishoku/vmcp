package securitydemo

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
)

func (s *Server) backend(w http.ResponseWriter, r *http.Request) {
	if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+s.Secret)) != 1 {
		http.Error(w, "unauthorized", 401)
		return
	}
	if r.Method == "GET" {
		http.Error(w, "no stream", 405)
		return
	}
	var req struct {
		ID     json.RawMessage `json:"id"`
		Method string          `json:"method"`
		Params json.RawMessage `json:"params"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req) != nil {
		http.Error(w, "invalid request", 400)
		return
	}
	if len(req.ID) == 0 {
		w.WriteHeader(202)
		return
	}
	var result any
	switch req.Method {
	case "initialize":
		result = map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{"tools": map[string]any{}, "resources": map[string]any{}, "prompts": map[string]any{}, "completions": map[string]any{}}, "serverInfo": map[string]string{"name": "security-demo", "version": "1"}}
	case "ping":
		result = map[string]any{}
	case "tools/list":
		tools := []any{}
		for _, name := range []string{"echo", "step_up", "delete_records", "export_records", "execution_counts"} {
			tools = append(tools, map[string]any{"name": name, "description": "Protocol lab: " + name, "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"dataset": map[string]string{"type": "string"}, "count": map[string]string{"type": "integer"}}, "additionalProperties": false}, "annotations": map[string]bool{"readOnlyHint": name == "echo" || name == "execution_counts", "destructiveHint": name == "delete_records"}})
		}
		result = map[string]any{"tools": tools}
	case "tools/call":
		var p struct {
			Name      string                       `json:"name"`
			Arguments json.RawMessage              `json:"arguments"`
			Meta      map[string]map[string]string `json:"_meta"`
		}
		if json.Unmarshal(req.Params, &p) != nil {
			http.Error(w, "invalid parameters", 400)
			return
		}
		s.mu.Lock()
		if values, exists := p.Meta["demo/customer"]; exists {
			if _, ok := s.callerCustomer(values); !ok {
				s.mu.Unlock()
				reply(w, map[string]any{"jsonrpc": "2.0", "id": req.ID, "error": map[string]any{"code": -32003, "message": "invalid customer session"}})
				return
			}
		}
		if s.executions == nil {
			s.executions = map[string]int{}
		}
		switch p.Name {
		case "echo", "step_up", "delete_records", "export_records":
			s.executions[p.Name]++
		case "execution_counts":
		default:
			s.mu.Unlock()
			reply(w, map[string]any{"jsonrpc": "2.0", "id": req.ID, "error": map[string]any{"code": -32602, "message": "unknown tool"}})
			return
		}
		raw, _ := json.Marshal(s.executions)
		s.mu.Unlock()
		result = map[string]any{"content": []any{map[string]string{"type": "text", "text": fmt.Sprintf("Execution counts: %s", raw)}}}
	case "resources/list":
		result = map[string]any{"resources": []any{map[string]string{"uri": "demo://guide", "name": "Protocol lab guide", "mimeType": "text/plain"}}}
	case "resources/templates/list":
		result = map[string]any{"resourceTemplates": []any{}}
	case "resources/read":
		result = map[string]any{"contents": []any{map[string]string{"uri": "demo://guide", "mimeType": "text/plain", "text": "Use execution_counts to verify that denied calls never execute."}}}
	case "prompts/list":
		result = map[string]any{"prompts": []any{map[string]any{"name": "review", "description": "Review a proposed operation", "arguments": []any{map[string]any{"name": "operation", "required": true}}}}}
	case "prompts/get":
		result = map[string]any{"messages": []any{map[string]any{"role": "user", "content": map[string]string{"type": "text", "text": "Review the exact operation details before approving."}}}}
	case "completion/complete":
		result = map[string]any{"completion": map[string]any{"values": []string{"demo", "test"}, "total": 2, "hasMore": false}}
	default:
		reply(w, map[string]any{"jsonrpc": "2.0", "id": req.ID, "error": map[string]any{"code": -32601, "message": "unsupported demo method"}})
		return
	}
	reply(w, map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": result})
}
