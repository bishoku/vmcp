package omni

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"

	"github.com/bishoku/vmcp/core/internal/mcp"
	"github.com/bishoku/vmcp/core/internal/netpolicy"
	"github.com/bishoku/vmcp/core/internal/ratelimit"
	"github.com/bishoku/vmcp/core/internal/telemetry"
)

type Server struct {
	Auditor           Auditor
	AllowAnonymous    bool
	SearchLimit       int
	limiter           *ratelimit.Local
	Backend           *Backend
	Index             *Index
	PublicURL, Issuer string
	TrustedProxies    []netip.Prefix
	slots             chan struct{}
}

func (s *Server) Handler() http.Handler {
	s.slots = make(chan struct{}, 64)
	s.limiter = ratelimit.NewLocal()
	if s.SearchLimit <= 0 {
		s.SearchLimit = 120
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if !s.Index.Status().Ready {
			http.Error(w, "index unavailable", 503)
			return
		}
		w.WriteHeader(200)
	})
	mux.HandleFunc("GET /.well-known/oauth-protected-resource/mcp/v1/omni", func(w http.ResponseWriter, r *http.Request) {
		if s.Issuer == "" || s.PublicURL == "" {
			http.NotFound(w, r)
			return
		}
		replyJSON(w, map[string]any{"resource": strings.TrimRight(s.PublicURL, "/") + "/mcp/v1/omni", "authorization_servers": []string{s.Issuer}, "bearer_methods_supported": []string{"header"}})
	})
	mux.HandleFunc("POST /mcp/v1/omni", s.post)
	mux.HandleFunc("GET /internal/status", s.admin(func(w http.ResponseWriter, r *http.Request) { replyJSON(w, s.Index.Status()) }))
	mux.HandleFunc("POST /internal/reindex", s.admin(func(w http.ResponseWriter, r *http.Request) {
		s.Index.Reindex()
		w.WriteHeader(202)
		replyJSON(w, map[string]bool{"accepted": true})
	}))
	mux.HandleFunc("POST /internal/preview", s.admin(s.preview))
	metrics := telemetry.New()
	metrics.Gauges = func(context.Context) map[string]float64 {
		status := s.Index.Status()
		ready := 0.0
		if status.Ready {
			ready = 1
		}
		rerankEnabled := 0.0
		if status.RerankEnabled {
			rerankEnabled = 1
		}
		return map[string]float64{"vmcp_omni_reranker_enabled": rerankEnabled, "vmcp_omni_index_documents": float64(status.Documents), "vmcp_omni_index_ready": ready, "vmcp_omni_index_revision": float64(status.Revision), "vmcp_omni_index_embedded_documents": float64(status.EmbeddedDocuments), "vmcp_omni_index_dimensions": float64(status.Dimensions)}
	}
	mux.Handle("GET /metrics", metrics)
	return metrics.Handler(mux)
}
func (s *Server) admin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-VMCP-Omni-Secret")), []byte(s.Backend.Secret)) != 1 {
			http.NotFound(w, r)
			return
		}
		next(w, r)
	}
}
func replyJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, err error) {
	var e *HTTPError
	status := 503
	if errors.As(err, &e) {
		status = e.Status
	}
	if status == 401 && w.Header().Get("WWW-Authenticate") == "" {
		w.Header().Set("WWW-Authenticate", `Bearer realm="vmcp"`)
	}
	http.Error(w, http.StatusText(status), status)
}
func (s *Server) ip(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return ""
	}
	trusted := func(a netip.Addr) bool {
		for _, p := range s.TrustedProxies {
			if p.Contains(a.Unmap()) {
				return true
			}
		}
		return false
	}
	if trusted(ip) {
		parts := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
		for i := len(parts) - 1; i >= 0; i-- {
			a, e := netip.ParseAddr(strings.TrimSpace(parts[i]))
			if e != nil {
				return ""
			}
			ip = a
			if !trusted(ip) {
				break
			}
		}
	}
	return ip.Unmap().String()
}
func (s *Server) access(ctx context.Context, token, ip string, full bool) (Access, error) {
	if !(s.AllowAnonymous && token == "") && (!strings.HasPrefix(token, "Bearer ") || len(token) <= 7) {
		return Access{}, &HTTPError{401}
	}
	var a Access
	path := "/internal/omni/access"
	if !full {
		path += "?check_only=1"
	}
	err := s.Backend.Request(ctx, "GET", path, token, ip, "", nil, &a)
	return a, err
}

type query struct {
	Query     string          `json:"query"`
	Server    string          `json:"server_id"`
	Project   string          `json:"project"`
	Limit     int             `json:"limit"`
	Ref       string          `json:"tool_ref"`
	Arguments json.RawMessage `json:"arguments"`
	Version   string          `json:"schema_version"`
}

func (q *query) validateSearch() bool {
	if q.Limit == 0 {
		q.Limit = 10
	}
	return len(strings.TrimSpace(q.Query)) > 0 && len(q.Query) <= 4096 && q.Limit > 0 && q.Limit <= 50
}
func (s *Server) post(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	origins, err := netpolicy.Origins()
	if err != nil {
		http.Error(w, "invalid ingress configuration", 503)
		return
	}
	if values, ok := r.Header["Origin"]; ok {
		if len(values) != 1 {
			http.Error(w, "origin denied", 403)
			return
		}
		origin, e := netpolicy.CanonicalOrigin(values[0])
		if e != nil || !origins[origin] {
			http.Error(w, "origin denied", 403)
			return
		}
	}
	media, _, mediaErr := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if mediaErr != nil || media != "application/json" {
		http.Error(w, "JSON required", 415)
		return
	}
	if v := r.Header.Get("MCP-Protocol-Version"); v != "" && !mcp.SupportsVersion(v) {
		http.Error(w, "unsupported protocol version", 400)
		return
	}
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	default:
		http.Error(w, "Omni concurrency limit exceeded", 429)
		return
	}
	token, ip := r.Header.Get("Authorization"), s.ip(r)
	raw, readErr := io.ReadAll(http.MaxBytesReader(w, r.Body, 4<<20))
	if readErr != nil {
		http.Error(w, "invalid request", 400)
		return
	}
	msg, err := mcp.Decode(raw)
	if err != nil {
		http.Error(w, "invalid JSON-RPC", 400)
		return
	}
	if msg.IsNotification() {
 if _,err:=s.access(r.Context(),token,ip,false);err!=nil{s.fail(w,err);return}
		w.WriteHeader(202)
		return
	}
	if !msg.IsRequest() {
		http.Error(w, "request required", 400)
		return
	}
	if msg.Method == mcp.MethodToolsCall {
		var call mcp.CallToolParams
		if json.Unmarshal(msg.Params, &call) == nil && (call.Name == "discover_server" || call.Name == "discover_tools" || call.Name == "get_tool_schema" || call.Name == "invoke_tool") {
			original := w
			ctx, complete, err := s.beginAudit(r.Context(), token, ip, r.Header.Get("traceparent"), &MessageAudit{Tool: call.Name, Payload: raw})
			if err != nil {
				http.Error(w, "audit admission unavailable", 503)
				return
			}
			r = r.WithContext(ctx)
			if complete != nil {
				buffer := &auditedResponse{header: http.Header{}}
				w = buffer
				defer complete(original, buffer)
			}
		}
	}
	_, err = s.access(r.Context(), token, ip, false)
	if err != nil {
		s.fail(w, err)
		return
	}

	rpcErr := func(code int, message string) { replyJSON(w, mcp.NewErrorResponse(msg.ID, code, message)) }
	result := func(v any) { res, _ := mcp.NewResult(msg.ID, v); replyJSON(w, res) }
	switch msg.Method {
	case mcp.MethodInitialize:
		p, e := mcp.ValidateInitializeParams(msg.Params)
		if e != nil {
			rpcErr(mcp.CodeInvalidParams, "invalid initialize parameters")
			return
		}
		result(mcp.InitializeResult{ProtocolVersion: mcp.NegotiateVersion(p.ProtocolVersion), Capabilities: mcp.ServerCapabilities{Tools: &mcp.ToolsCapability{}, Resources: &mcp.ResourcesCapability{}}, ServerInfo: mcp.Implementation{Name: "vmcp-omni", Version: "0.1.0"}, Instructions: initializeInstructions})
	case mcp.MethodPing:
		result(struct{}{})
	case "resources/list", "resources/templates/list":
		var params struct {
			Cursor string `json:"cursor"`
		}
		if (len(msg.Params) > 0 && json.Unmarshal(msg.Params, &params) != nil) || params.Cursor != "" {
			rpcErr(mcp.CodeInvalidParams, "invalid resource list parameters; this catalog has one page")
			return
		}
		if msg.Method == "resources/templates/list" {
			result(map[string]any{"resourceTemplates": []any{}})
		} else {
			result(map[string]any{"resources": workflowResources()})
		}
	case "resources/read":
		var params struct {
			URI string `json:"uri"`
		}
		if json.Unmarshal(msg.Params, &params) != nil || strings.TrimSpace(params.URI) == "" {
			rpcErr(mcp.CodeInvalidParams, "resource uri required")
			return
		}
		if params.URI != WorkflowGuideURI {
			rpcErr(-32002, "resource not found")
			return
		}
		result(map[string]any{"contents": []map[string]string{{"uri": WorkflowGuideURI, "mimeType": "text/markdown", "text": workflowGuide}}})
	case mcp.MethodToolsList:
		result(mcp.ListToolsResult{Tools: Tools()})
	case mcp.MethodToolsCall:
		var call mcp.CallToolParams
		var q query
		if json.Unmarshal(msg.Params, &call) != nil || json.Unmarshal(call.Arguments, &q) != nil {
			rpcErr(mcp.CodeInvalidParams, "invalid tool arguments")
			return
		}
		switch call.Name {
		case "discover_server", "discover_tools":
			if !s.allowSearch(w, r, token) {
				return
			}
			if !q.validateSearch() {
				rpcErr(mcp.CodeInvalidParams, "query required (maximum 4096 bytes), limit must be 1–50")
				return
			}
			search := s.Index.Search
			if call.Name == "discover_server" {
				search = s.Index.SearchServers
			}
			access, e := s.access(r.Context(), token, ip, true)
			if e != nil {
				s.fail(w, e)
				return
			}
			searchCtx := context.WithValue(r.Context(), accessRefreshKey{}, accessRefresh(func(ctx context.Context) (Access, error) { return s.access(ctx, token, ip, true) }))
			hits, mode, e := search(searchCtx, access, q.Query, q.Server, q.Project, q.Limit)
			if e != nil {
				s.fail(w, e)
				return
			}
			fresh, e := s.access(r.Context(), token, ip, true)
			if e != nil {
				s.fail(w, e)
				return
			}
			if trace := traceFrom(r.Context()); trace != nil {
				trace.add(TraceStage{Name: "search_mode", Status: mode})
			}
			before := len(hits)
			hits = authorizedHits(hits, fresh)
			if trace := traceFrom(r.Context()); trace != nil {
				trace.Counts["final_acl_removed"] = before - len(hits)
			}
			traceHits(r.Context(), "final_results", hits)
			if call.Name == "discover_server" {
				servers := []map[string]any{}
				seen := map[string]bool{}
				for _, h := range hits {
					if seen[h.Server] {
						continue
					}
					seen[h.Server] = true
					servers = append(servers, map[string]any{"server_id": h.Server, "name": h.ServerName, "description": h.ServerDescription, "project": h.Project, "score": h.Score})
					if len(servers) >= q.Limit {
						break
					}
				}
				traceCount(r.Context(), "returned_servers", len(servers))
				result(toolResult(map[string]any{"servers": servers, "search_mode": mode}))
				return
			}
			traceCount(r.Context(), "returned_tools", len(hits))
			result(toolResult(map[string]any{"tools": hits, "search_mode": mode}))
		case "get_tool_schema", "invoke_tool":
			if _, _, e := ParseRef(q.Ref); e != nil {
				rpcErr(mcp.CodeInvalidParams, "invalid tool_ref")
				return
			}
			path := "/internal/omni/schema"
			if call.Name == "invoke_tool" {
				var args map[string]json.RawMessage
				if json.Unmarshal(q.Arguments, &args) != nil || args == nil {
					rpcErr(mcp.CodeInvalidParams, "arguments must be an object")
					return
				}
				path = "/internal/omni/invoke"
			}
			if call.Name == "get_tool_schema" {
				var schema Schema
				if e := s.Backend.Request(r.Context(), "POST", path, token, ip, r.Header.Get("traceparent"), q, &schema); e != nil {
					s.fail(w, e)
					return
				}
				result(toolResult(schema))
				return
			}
			var response mcp.Message
			if e := s.Backend.Request(r.Context(), "POST", path, token, ip, r.Header.Get("traceparent"), q, &response); e != nil {
				s.fail(w, e)
				return
			}
			response.ID = msg.ID
			replyJSON(w, &response)
		default:
			rpcErr(mcp.CodeInvalidParams, "unknown Omni tool")
		}
	default:
		rpcErr(mcp.CodeMethodNotFound, "method not supported")
	}
}
func toolResult(v any) any {
	b, _ := json.Marshal(v)
	return map[string]any{"content": []map[string]string{{"type": "text", "text": string(b)}}, "structuredContent": v, "isError": false}
}
func Tools() []mcp.Tool {
	search := json.RawMessage(`{"type":"object","properties":{"query":{"type":"string","minLength":1,"maxLength":4096,"description":"Describe the task goal for server discovery or the needed capability for tool discovery."},"server_id":{"type":"string","description":"Exact server_id returned by discover_server. Normally set this when discovering tools for a selected server."},"project":{"type":"string","description":"Optional exact project identifier. Preserve an explicit user project scope across discovery calls."},"limit":{"type":"integer","minimum":1,"maximum":50,"default":10}},"required":["query"],"additionalProperties":false}`)
	return []mcp.Tool{
		{Name: "discover_server", Description: "Start a new task here to find authorized virtual MCP servers matching the overall goal. Select a suitable server_id, then pass it to discover_tools. Keep the selection for the task; if servers are equally suitable, prefer the lexicographically smallest server_id. Returns metadata, not tool schemas.", InputSchema: search},
		{Name: "discover_tools", Description: "Find authorized tools for a capability within the server_id selected with discover_server. Normally include server_id and preserve explicit project scope to avoid duplicate routes across virtual servers. Unscoped search remains available when selection is unresolved. If needed capabilities are missing, consider another server. Fetch get_tool_schema before invocation.", InputSchema: search},
		{Name: "get_tool_schema", Description: "Before invocation, read the current published input/output schema and annotations for a tool_ref returned by discovery. Treat the reference as opaque. Use the returned schema_version with the same reference in invoke_tool; do not guess arguments or reuse another tool's version.", InputSchema: json.RawMessage(`{"type":"object","properties":{"tool_ref":{"type":"string"}},"required":["tool_ref"],"additionalProperties":false}`)},
		{Name: "invoke_tool", Description: "Invoke the selected tool_ref with schema-valid arguments and the schema_version returned by get_tool_schema. Respect annotations and client/user approval policies. The selected server determines execution and audit context. Do not automatically retry a failed or uncertain invocation or switch servers to repeat it; a timed-out write may already have executed.", InputSchema: json.RawMessage(`{"type":"object","properties":{"tool_ref":{"type":"string"},"arguments":{"type":"object"},"schema_version":{"type":"string"}},"required":["tool_ref","arguments"],"additionalProperties":false}`), Annotations: json.RawMessage(`{"readOnlyHint":false,"destructiveHint":true,"idempotentHint":false,"openWorldHint":true}`)},
	}
}
func (s *Server) preview(w http.ResponseWriter, r *http.Request) {
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	default:
		http.Error(w, "Omni concurrency limit exceeded", 429)
		return
	}

	var p struct {
		Query string `json:"query"`
		Ref   string `json:"tool_ref"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10)).Decode(&p) != nil || len(p.Query) > 4096 {
		http.Error(w, "invalid preview", 400)
		return
	}
	ip := r.Header.Get("X-VMCP-Client-IP")
	if _, e := netip.ParseAddr(ip); e != nil {
		http.Error(w, "client address required", 400)
		return
	}
	a, err := s.access(r.Context(), r.Header.Get("Authorization"), ip, true)
	if err != nil {
		s.fail(w, err)
		return
	}
	if p.Ref != "" {
		if _, ok := a.Refs[p.Ref]; !ok {
			s.fail(w, &HTTPError{403})
			return
		}
		var schema Schema
		err := s.Backend.Request(r.Context(), "POST", "/internal/omni/schema", r.Header.Get("Authorization"), ip, "", map[string]string{"tool_ref": p.Ref}, &schema)
		if err != nil {
			s.fail(w, err)
			return
		}
		replyJSON(w, schema)
		return
	}
	if strings.TrimSpace(p.Query) == "" {
		http.Error(w, "query required", 400)
		return
	}
	if !s.allowSearch(w, r, r.Header.Get("Authorization")) {
		return
	}
	searchCtx := context.WithValue(r.Context(), accessRefreshKey{}, accessRefresh(func(ctx context.Context) (Access, error) {
		return s.access(ctx, r.Header.Get("Authorization"), ip, true)
	}))
	hits, mode, err := s.Index.Search(searchCtx, a, p.Query, "", "", 10)
	if err != nil {
		s.fail(w, err)
		return
	}
	fresh, err := s.access(r.Context(), r.Header.Get("Authorization"), ip, true)
	if err != nil {
		s.fail(w, err)
		return
	}
	replyJSON(w, map[string]any{"tools": authorizedHits(hits, fresh), "search_mode": mode})
}

func authorizedHits(hits []Hit, access Access) []Hit {
	out := make([]Hit, 0, len(hits))
	for _, h := range hits {
		if access.Refs[h.Ref] == h.Document.Hash() {
			out = append(out, h)
		}
	}
	return out
}

func (s *Server) allowSearch(w http.ResponseWriter, r *http.Request, token string) bool {
	identity := "credential:" + token
	if token == "" {
		ip := s.ip(r)
		if verified := r.Header.Get("X-VMCP-Client-IP"); r.URL.Path == "/internal/preview" && verified != "" {
			ip = verified
		}
		if ip == "" {
			http.Error(w, "client address required", 400)
			return false
		}
		identity = "anonymous:" + ip
	}
	hash := sha256.Sum256([]byte(identity))
	ok, retry, _ := s.limiter.Allow(r.Context(), fmt.Sprintf("%x", hash), s.SearchLimit)
	if !ok {
		w.Header().Set("Retry-After", strconv.Itoa(max(1, int(retry.Seconds()+1))))
		http.Error(w, "discovery rate limit exceeded", 429)
	}
	return ok
}
func (s *Server) fail(w http.ResponseWriter, err error) {
	var e *HTTPError
	if errors.As(err, &e) && e.Status == 401 && s.Issuer != "" && s.PublicURL != "" {
		w.Header().Set("WWW-Authenticate", `Bearer realm="vmcp", resource_metadata="`+strings.TrimRight(s.PublicURL, "/")+`/.well-known/oauth-protected-resource/mcp/v1/omni"`)
	}
	fail(w, err)
}
