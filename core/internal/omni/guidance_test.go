package omni

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/bishoku/vmcp/core/internal/mcp"
)

func guidanceServer(t *testing.T, anonymous bool) (http.Handler, *atomic.Int32) {
	t.Helper()
	status := &atomic.Int32{}
	status.Store(200)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/omni/access" || r.URL.Query().Get("check_only") != "1" {
			t.Errorf("static guidance requested a catalog or execution endpoint: %s", r.URL)
			w.WriteHeader(500)
			return
		}
		if r.Header.Get("Authorization") != "Bearer valid" && !(anonymous && r.Header.Get("Authorization") == "") {
			w.WriteHeader(401)
			return
		}
		if code := int(status.Load()); code != 200 {
			w.WriteHeader(code)
			return
		}
		_ = json.NewEncoder(w).Encode(Access{})
	}))
	t.Cleanup(backend.Close)
	b, err := NewBackend(backend.URL, strings.Repeat("s", 32))
	if err != nil {
		t.Fatal(err)
	}
	// Static guidance is useful even while the search index is not ready.
	return (&Server{Backend: b, Index: NewIndex(nil), AllowAnonymous: anonymous}).Handler(), status
}

func guidanceRPC(t *testing.T, handler http.Handler, token, method string, params any) *httptest.ResponseRecorder {
	t.Helper()
	request := map[string]any{"jsonrpc": "2.0", "id": 7, "method": method}
	if params != nil {
		request["params"] = params
	}
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "/mcp/v1/omni", strings.NewReader(string(body)))
	r.Header.Set("Content-Type", "application/json")
	if token != "" {
		r.Header.Set("Authorization", token)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	return w
}

func guidanceResult(t *testing.T, w *httptest.ResponseRecorder, out any) {
	t.Helper()
	var envelope struct {
		Result json.RawMessage `json:"result"`
		Error  json.RawMessage `json:"error"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &envelope) != nil || len(envelope.Error) > 0 {
		t.Fatalf("RPC failed: %d %s", w.Code, w.Body.String())
	}
	if err := json.Unmarshal(envelope.Result, out); err != nil {
		t.Fatal(err)
	}
}

func TestOmniAdvertisesGuidanceWithoutPromptsOrExtraTools(t *testing.T) {
	h, _ := guidanceServer(t, false)
	var init mcp.InitializeResult
	guidanceResult(t, guidanceRPC(t, h, "Bearer valid", "initialize", map[string]any{
		"protocolVersion": mcp.ProtocolVersion20250618, "capabilities": map[string]any{},
		"clientInfo": map[string]string{"name": "guidance-test", "version": "1"},
	}), &init)
	if init.Capabilities.Resources == nil || init.Capabilities.Tools == nil || init.Capabilities.Prompts != nil {
		t.Fatalf("unexpected capabilities: %+v", init.Capabilities)
	}
	if init.Capabilities.Resources.Subscribe || init.Capabilities.Resources.ListChanged {
		t.Fatal("static resource advertised unsupported subscriptions/notifications")
	}
	for _, want := range []string{WorkflowGuideURI, "discover_server", "server_id", "get_tool_schema", "schema_version", "Do not automatically retry"} {
		if !strings.Contains(init.Instructions, want) {
			t.Fatalf("initialize instructions omit %q", want)
		}
	}
	var tools mcp.ListToolsResult
	guidanceResult(t, guidanceRPC(t, h, "Bearer valid", "tools/list", nil), &tools)
	if len(tools.Tools) != 4 {
		t.Fatalf("guidance changed tool count: %d", len(tools.Tools))
	}
	if !strings.Contains(tools.Tools[1].Description, "server_id") {
		t.Fatal("scoped discovery guidance missing")
	}
	var schema struct {
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(tools.Tools[1].InputSchema, &schema); err != nil {
		t.Fatal(err)
	}
	if len(schema.Required) != 1 || schema.Required[0] != "query" {
		t.Fatal("guidance made scoped discovery mandatory")
	}
}

func TestOmniListsAndReadsEmbeddedEnglishWorkflow(t *testing.T) {
	h, _ := guidanceServer(t, false)
	var list struct {
		Resources []struct {
			URI      string `json:"uri"`
			MimeType string `json:"mimeType"`
		} `json:"resources"`
		NextCursor string `json:"nextCursor"`
	}
	guidanceResult(t, guidanceRPC(t, h, "Bearer valid", "resources/list", map[string]any{}), &list)
	if len(list.Resources) != 1 || list.Resources[0].URI != WorkflowGuideURI || list.Resources[0].MimeType != "text/markdown" || list.NextCursor != "" {
		t.Fatalf("invalid resource list: %+v", list)
	}
	var read struct {
		Contents []struct {
			URI, MimeType, Text string
		} `json:"contents"`
	}
	guidanceResult(t, guidanceRPC(t, h, "Bearer valid", "resources/read", map[string]string{"uri": list.Resources[0].URI}), &read)
	if len(read.Contents) != 1 || read.Contents[0].Text != workflowGuide || read.Contents[0].URI != WorkflowGuideURI || read.Contents[0].MimeType != "text/markdown" {
		t.Fatal("resource contents did not match embedded guide")
	}
	for _, want := range []string{"# Omni MCP discovery workflow", "## 1. Select a server", "## 2. Discover tools", "## 3. Read", "## 4. Invoke", "Do not automatically retry", "does not remove global search"} {
		if !strings.Contains(read.Contents[0].Text, want) {
			t.Fatalf("missing workflow requirement: %s", want)
		}
	}
	var templates struct {
		Templates []any `json:"resourceTemplates"`
	}
	guidanceResult(t, guidanceRPC(t, h, "Bearer valid", "resources/templates/list", nil), &templates)
	if templates.Templates == nil || len(templates.Templates) != 0 {
		t.Fatal("unexpected templates")
	}
}

func TestOmniGuidanceErrorsDoNotFetchArbitraryResources(t *testing.T) {
	h, _ := guidanceServer(t, false)
	for _, tc := range []struct {
		method string
		params any
		code   int
	}{
		{"resources/read", map[string]string{"uri": "https://example.invalid/private"}, -32002},
		{"resources/read", map[string]string{"uri": "omni://guides/discovery-workflow/v2"}, -32002},
		{"resources/read", map[string]string{}, mcp.CodeInvalidParams},
		{"resources/read", map[string]int{"uri": 2}, mcp.CodeInvalidParams},
		{"resources/list", map[string]string{"cursor": "unexpected"}, mcp.CodeInvalidParams},
		{"resources/templates/list", map[string]string{"cursor": "unexpected"}, mcp.CodeInvalidParams},
		{"resources/subscribe", map[string]string{"uri": WorkflowGuideURI}, mcp.CodeMethodNotFound},
		{"prompts/list", nil, mcp.CodeMethodNotFound},
		{"prompts/get", map[string]string{"name": "workflow"}, mcp.CodeMethodNotFound},
	} {
		t.Run(tc.method+"/"+stringMustJSON(tc.params), func(t *testing.T) {
			w := guidanceRPC(t, h, "Bearer valid", tc.method, tc.params)
			var response struct {
				Error struct{ Code int } `json:"error"`
			}
			if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &response) != nil || response.Error.Code != tc.code {
				t.Fatalf("wrong RPC error: %s", w.Body.String())
			}
			if strings.Contains(w.Body.String(), "# Omni MCP") {
				t.Fatal("error response contained resource content")
			}
		})
	}
}

func stringMustJSON(v any) string { b, _ := json.Marshal(v); return string(b) }

func TestOmniGuidanceUsesExistingAuthorizationAndRevocation(t *testing.T) {
	for _, anonymous := range []bool{false, true} {
		t.Run(stringMustJSON(anonymous), func(t *testing.T) {
			h, status := guidanceServer(t, anonymous)
			for _, method := range []string{"resources/list", "resources/read"} {
				params := map[string]string{"uri": WorkflowGuideURI}
				for _, token := range []string{"", "Bearer invalid", "Bearer valid"} {
					want := 200
					if token == "Bearer invalid" || token == "" && !anonymous {
						want = 401
					}
					if got := guidanceRPC(t, h, token, method, params); got.Code != want {
						t.Fatalf("anonymous=%v token=%q method=%s status=%d want=%d", anonymous, token, method, got.Code, want)
					}
				}
				for _, code := range []int32{401, 503} {
					status.Store(code)
					w := guidanceRPC(t, h, "Bearer valid", method, params)
					if w.Code != int(code) || strings.Contains(w.Body.String(), "# Omni MCP") {
						t.Fatalf("revocation/outage exposed guidance: %d %s", w.Code, w.Body.String())
					}
				}
				status.Store(200)
			}
		})
	}
}
