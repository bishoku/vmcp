package virtual

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/bishoku/vmcp/core/internal/audit"
	"github.com/bishoku/vmcp/core/internal/config"
	"github.com/bishoku/vmcp/core/internal/mcp"
	"github.com/bishoku/vmcp/core/internal/pii"
	"github.com/bishoku/vmcp/core/internal/restapi"
	"github.com/bishoku/vmcp/core/internal/secrets"
	"github.com/bishoku/vmcp/core/internal/store"
	"github.com/bishoku/vmcp/core/internal/testdb"
)

// Use the same sample and inferred schema as the visual editor regression test.
func TestRESTOrderMappingThroughVirtualMCP(t *testing.T) {
	raw, err := os.ReadFile("../../../web/src/connections/testdata/order-mapping.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct{ Body, BodySchema json.RawMessage }
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	wrapped := `{"body":` + string(fixture.Body) + `}`
	for _, tc := range []struct {
		name, method, path, args, wantBody, wantError, response string
		status, calls                                           int
		statuses                                                []int
		audit                                                   bool
		maskArguments                                           bool
	}{
		{name: "create nested order", args: wrapped, calls: 1},
		{name: "create with audit capture", args: wrapped, calls: 1, audit: true},
		{name: "PII argument masking intentionally changes upstream values", args: wrapped, calls: 1, maskArguments: true, wantBody: strings.ReplaceAll(string(fixture.Body), "CUST-42", "[REDACTED:customer]")},
		{name: "update separates path query and body", method: "PUT", path: "/orders/{id}", args: `{"id":"ORD-42","notify":false,"body":` + string(fixture.Body) + `}`, calls: 1},
		{name: "missing body wrapper rejected before dispatch", args: string(fixture.Body), wantError: "arguments do not match", calls: 0},
		{name: "stringified body rejected before dispatch", args: `{"body":"{\"order\":{}}"}`, wantError: "arguments do not match", calls: 0},
		{name: "stringified items rejected before dispatch", args: `{"body":{"order":{"items":"[]"}}}`, wantError: "arguments do not match", calls: 0},
		{name: "quantity string is not coerced", args: `{"body":{"order":{"items":[{"productId":"SKU-1","quantity":"2"}]}}}`, wantError: "arguments do not match", calls: 0},
		{name: "optional inferred fields can reach upstream incomplete", args: `{"body":{"order":{"items":[{"quantity":2}]}}}`, wantBody: `{"order":{"items":[{"quantity":2}]}}`, status: 422, response: `{"error":"productId is required"}`, wantError: "REST service returned HTTP 422", calls: 1},
		{name: "field names are not renamed", args: `{"body":{"order":{"items":[{"productid":"SKU-1","quantity":2}]}}}`, wantBody: `{"order":{"items":[{"productid":"SKU-1","quantity":2}]}}`, status: 422, wantError: "REST service returned HTTP 422", calls: 1},
		{name: "unexpected success status after dispatch", args: wrapped, statuses: []int{200}, wantError: "REST service returned HTTP 201", calls: 1},
		{name: "invalid response schema after dispatch", args: wrapped, response: `{"id":42}`, wantError: "response does not match", calls: 1},
		{name: "empty created response after dispatch", args: wrapped, response: " ", wantError: "invalid JSON", calls: 1},
		{name: "no content conflicts with object response schema", args: wrapped, status: 204, wantError: "response does not match", calls: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.method == "" {
				tc.method = "POST"
			}
			if tc.path == "" {
				tc.path = "/orders"
			}
			if tc.status == 0 {
				tc.status = 201
			}
			if tc.response == "" {
				tc.response = `{"id":"ORD-42"}`
			}
			if tc.wantBody == "" {
				tc.wantBody = string(fixture.Body)
			}
			var calls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				wantPath := "/api/orders"
				if tc.method == "PUT" {
					wantPath += "/ORD-42"
				}
				if r.Method != tc.method || r.URL.Path != wantPath || r.Header.Get("Content-Type") != "application/json" {
					t.Errorf("wrong HTTP request: %s %s %v", r.Method, r.URL, r.Header)
				}
				if tc.method == "PUT" && r.URL.RawQuery != "notify=false" {
					t.Errorf("query mismatch: %s", r.URL.RawQuery)
				}
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Error(err)
				}
				var got, want any
				if json.Unmarshal(body, &got) != nil || json.Unmarshal([]byte(tc.wantBody), &want) != nil || !reflect.DeepEqual(got, want) {
					t.Errorf("HTTP body mismatch: got %s want %s", body, tc.wantBody)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				if tc.status != 204 {
					_, _ = w.Write([]byte(tc.response))
				}
			}))
			defer srv.Close()
			op := restapi.Operation{Name: "create_order", Enabled: true, Method: tc.method, Path: tc.path, BodySchema: fixture.BodySchema, ResponseSchema: json.RawMessage(`{"type":"object","properties":{"id":{"type":"string"}},"required":["id"]}`), SuccessStatuses: tc.statuses}
			if tc.method == "PUT" {
				op.Parameters = []restapi.Parameter{{Name: "id", In: "path", Schema: json.RawMessage(`{"type":"string"}`)}, {Name: "notify", In: "query", Schema: json.RawMessage(`{"type":"boolean"}`)}}
			}
			compiled, err := restapi.Compile(srv.URL+"/api", restapi.Config{Auth: restapi.Auth{Type: "none"}, AllowInsecureHTTP: true, TimeoutSeconds: 3, Operations: []restapi.Operation{op}}, nil)
			if err != nil {
				t.Fatal(err)
			}
			r := newRig(t)
			snap := r.handler.Snap.(staticSnap).s
			snap.Upstreams["u-orders"] = config.Upstream{ID: "u-orders", Slug: "orders", URL: srv.URL + "/api", REST: compiled}
			tool := compiled.Tools[0]
			tool.Name = "orders__create_order"
			snap.VirtualServers["support"] = config.VirtualServer{Slug: "support", Tools: []config.VirtualTool{{UpstreamID: "u-orders", UpstreamName: "create_order", Tool: tool}}}
			if tc.maskArguments {
				masker, err := pii.Compile(pii.Config{Enabled: true, MaskArguments: true, Detectors: []string{"email"}, CustomRules: []pii.Rule{{Name: "customer", Pattern: `CUST-[0-9]+`}}})
				if err != nil {
					t.Fatal(err)
				}
				vs := snap.VirtualServers["support"]
				vs.PII = masker
				snap.VirtualServers["support"] = vs
			}
			if tc.audit {
				ctx := context.Background()
				dbURL := testdb.New(t)
				if err := store.Migrate(ctx, dbURL); err != nil {
					t.Fatal(err)
				}
				pool, err := store.Connect(ctx, dbURL)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(pool.Close)
				box, err := secrets.NewBox(base64.StdEncoding.EncodeToString(make([]byte, 32)))
				if err != nil {
					t.Fatal(err)
				}
				writerCtx, cancel := context.WithCancel(ctx)
				runtime := audit.NewRuntime(writerCtx, pool, box, r.handler.Log, "strict")
				t.Cleanup(func() { cancel(); <-runtime.Done() })
				snap.Audit = audit.DefaultSettings()
				snap.Audit.Scopes["virtual"] = audit.Capture{Request: true, Response: true}
				snap.Audit.Scopes["upstream"] = audit.Capture{Request: true, Response: true}
				snap.Audit.Rules = []audit.Rule{{Name: "product", Pattern: `SKU-[0-9]+`}}
				runtime.SetSettings(snap.Audit)
				r.handler.Audit = runtime
			}
			sid := r.initialize(t)
			_, listed := r.rpc(t, "support", sid, mcp.MethodToolsList, nil)
			if listed == nil || listed.Error != nil {
				t.Fatalf("tools/list failed: %+v", listed)
			}
			var list mcp.ListToolsResult
			if err := json.Unmarshal(listed.Result, &list); err != nil || len(list.Tools) != 1 {
				t.Fatalf("tools/list: %s %v", listed.Result, err)
			}
			var gotSchema, wantSchema any
			_ = json.Unmarshal(list.Tools[0].InputSchema, &gotSchema)
			_ = json.Unmarshal(tool.InputSchema, &wantSchema)
			if !reflect.DeepEqual(gotSchema, wantSchema) {
				t.Fatal("virtual server changed the advertised input schema")
			}
			preview, previewErr := compiled.Preview(srv.URL+"/api", "create_order", json.RawMessage(tc.args), nil)
			if tc.calls == 0 && previewErr == nil {
				t.Fatal("preview accepted invalid input")
			}
			if tc.calls > 0 {
				var got, want any
				_ = json.Unmarshal(preview.Body, &got)
				previewBody := tc.wantBody
				if tc.maskArguments {
					previewBody = string(fixture.Body)
				}
				_ = json.Unmarshal([]byte(previewBody), &want)
				if previewErr != nil || !reflect.DeepEqual(got, want) {
					t.Fatalf("preview differs from expected wire body: %s %v", preview.Body, previewErr)
				}
			}
			if calls.Load() != 0 {
				t.Fatal("listing or preview executed the request")
			}
			_, response := r.rpc(t, "support", sid, mcp.MethodToolsCall, mcp.CallToolParams{Name: tool.Name, Arguments: json.RawMessage(tc.args)})
			text := callText(t, response)
			var result struct {
				IsError bool `json:"isError"`
			}
			if err := json.Unmarshal(response.Result, &result); err != nil {
				t.Fatal(err)
			}
			if result.IsError != (tc.wantError != "") || (tc.wantError != "" && !strings.Contains(text, tc.wantError)) {
				t.Fatalf("unexpected result: %s", response.Result)
			}
			if calls.Load() != int32(tc.calls) {
				t.Fatalf("dispatch/replay count: got %d want %d", calls.Load(), tc.calls)
			}
		})
	}
}

func TestNestedResponseProjectionThroughVirtualMCP(t *testing.T) {
	t.Setenv("VMCP_EGRESS_ALLOW_CIDRS", "127.0.0.0/8")
	raw, err := os.ReadFile("../../../web/src/connections/testdata/response-projection.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []struct {
		Name                                   string
		Schema, Body, Expected, ExpectedSchema json.RawMessage
		Paths                                  [][]string
	}
	if err := json.Unmarshal(raw, &fixtures); err != nil {
		t.Fatal(err)
	}
	for _, fixture := range fixtures {
		t.Run(fixture.Name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write(fixture.Body)
			}))
			defer srv.Close()
			compiled, err := restapi.Compile(srv.URL, restapi.Config{Auth: restapi.Auth{Type: "none"}, AllowInsecureHTTP: true, TimeoutSeconds: 3, Operations: []restapi.Operation{{Name: "lookup", Enabled: true, Method: "GET", Path: "/", ResponseSchema: fixture.Schema, ResponseFieldPaths: fixture.Paths}}}, nil)
			if err != nil {
				t.Fatal(err)
			}
			r := newRig(t)
			snap := r.handler.Snap.(staticSnap).s
			snap.Upstreams["rest"] = config.Upstream{ID: "rest", Slug: "rest", URL: srv.URL, REST: compiled}
			tool := compiled.Tools[0]
			tool.Name = "rest__lookup"
			snap.VirtualServers["support"] = config.VirtualServer{Slug: "support", Tools: []config.VirtualTool{{UpstreamID: "rest", UpstreamName: "lookup", Tool: tool}}}
			sid := r.initialize(t)
			_, listed := r.rpc(t, "support", sid, mcp.MethodToolsList, nil)
			if listed == nil || listed.Error != nil {
				t.Fatalf("tools/list failed: %+v", listed)
			}
			var list mcp.ListToolsResult
			if err := json.Unmarshal(listed.Result, &list); err != nil || len(list.Tools) != 1 {
				t.Fatalf("tools/list: %s %v", listed.Result, err)
			}
			var shape struct{ Properties map[string]json.RawMessage }
			if err := json.Unmarshal(list.Tools[0].OutputSchema, &shape); err != nil {
				t.Fatal(err)
			}
			var gotSchema, wantSchema any
			_ = json.Unmarshal(shape.Properties["result"], &gotSchema)
			_ = json.Unmarshal(fixture.ExpectedSchema, &wantSchema)
			if !reflect.DeepEqual(gotSchema, wantSchema) {
				t.Fatalf("advertised output schema differs: %s", list.Tools[0].OutputSchema)
			}
			_, called := r.rpc(t, "support", sid, mcp.MethodToolsCall, mcp.CallToolParams{Name: tool.Name, Arguments: json.RawMessage(`{}`)})
			if called == nil || called.Error != nil {
				t.Fatalf("tools/call failed: %+v", called)
			}
			var result struct {
				IsError           bool
				StructuredContent struct{ Result any }
				Content           []struct{ Text string }
			}
			if err := json.Unmarshal(called.Result, &result); err != nil || result.IsError || len(result.Content) != 1 {
				t.Fatalf("call result: %s %v", called.Result, err)
			}
			var text struct{ Result any }
			var expected any
			_ = json.Unmarshal([]byte(result.Content[0].Text), &text)
			_ = json.Unmarshal(fixture.Expected, &expected)
			if !reflect.DeepEqual(result.StructuredContent.Result, expected) || !reflect.DeepEqual(text.Result, expected) {
				t.Fatalf("virtual output differs: %s", called.Result)
			}
		})
	}
}

func TestFixedAndDefaultParametersThroughVirtualMCP(t *testing.T) {
	t.Setenv("VMCP_EGRESS_ALLOW_CIDRS", "127.0.0.0/8")
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/sample" || r.URL.Query().Get("type") != "1" || r.URL.Query().Get("content") != "all & more" {
			t.Errorf("wrong upstream request: %s", r.URL)
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	hidden := false
	compiled, err := restapi.Compile(srv.URL, restapi.Config{Auth: restapi.Auth{Type: "none"}, TimeoutSeconds: 3, AllowInsecureHTTP: true, Operations: []restapi.Operation{{Name: "sample", Method: "GET", Path: "/sample", Enabled: true, Parameters: []restapi.Parameter{
		{Name: "type", In: "query", ExposeToMCP: &hidden, DefaultValue: json.RawMessage(`1`), Schema: json.RawMessage(`{"type":"integer"}`)},
		{Name: "content", In: "query", Required: true, DefaultValue: json.RawMessage(`"all & more"`), Schema: json.RawMessage(`{"type":"string"}`)},
	}}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	r := newRig(t)
	snap := r.handler.Snap.(staticSnap).s
	snap.Upstreams["rest"] = config.Upstream{ID: "rest", Slug: "rest", URL: srv.URL, REST: compiled}
	tool := compiled.Tools[0]
	tool.Name = "rest__sample"
	snap.VirtualServers["support"] = config.VirtualServer{Slug: "support", Tools: []config.VirtualTool{{UpstreamID: "rest", UpstreamName: "sample", Tool: tool}}}
	sid := r.initialize(t)
	_, listed := r.rpc(t, "support", sid, mcp.MethodToolsList, nil)
	if listed == nil || listed.Error != nil || strings.Contains(string(listed.Result), `"type":{"type":"integer"}`) {
		t.Fatalf("tools/list: %+v", listed)
	}
	_, called := r.rpc(t, "support", sid, mcp.MethodToolsCall, mcp.CallToolParams{Name: tool.Name, Arguments: json.RawMessage(`{}`)})
	if called == nil || called.Error != nil || strings.Contains(string(called.Result), `"isError":true`) || calls.Load() != 1 {
		t.Fatalf("default call: %+v count=%d", called, calls.Load())
	}
	_, called = r.rpc(t, "support", sid, mcp.MethodToolsCall, mcp.CallToolParams{Name: tool.Name, Arguments: json.RawMessage(`{"type":2}`)})
	if called == nil || !strings.Contains(string(called.Result), `"isError":true`) || calls.Load() != 1 {
		t.Fatalf("fixed value override was accepted: %+v", called)
	}
}
