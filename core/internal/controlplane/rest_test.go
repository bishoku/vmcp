package controlplane_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/bishoku/vmcp/core/internal/restapi"
	"github.com/bishoku/vmcp/core/internal/store"
)

func TestRESTLifecycleSecretsAndExplicitExecution(t *testing.T) {
	t.Setenv("VMCP_EGRESS_ALLOW_CIDRS", "127.0.0.0/8")
	a := setup(t)
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		u, p, ok := r.BasicAuth()
		if !ok || u != "service" || p != "rest-secret" || r.Header.Get("X-Operation") != "operation-secret" {
			t.Error("stored credentials missing")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()
	payload := map[string]any{"slug": "rest", "name": "REST", "kind": "rest", "url": srv.URL, "rest": map[string]any{"auth": map[string]any{"type": "basic", "username": "service", "password": "rest-secret"}, "allowInsecureHttp": true, "operations": []any{map[string]any{"name": "lookup", "method": "GET", "path": "/items", "enabled": true, "headers": map[string]string{"X-Operation": "operation-secret"}}}}}
	h, out := a.expect(201, "POST", "/api/v1/upstreams", payload)
	raw, _ := json.Marshal(out)
	if strings.Contains(string(raw), "rest-secret") || strings.Contains(string(raw), "operation-secret") {
		t.Fatal("secret leaked")
	}
	if out["kind"] != "rest" || out["toolCount"] != float64(1) || calls.Load() != 0 {
		t.Fatalf("%v calls=%d", out, calls.Load())
	}
	var ciphertext []byte
	if err := a.pool.QueryRow(context.Background(), `SELECT encrypted_rest_config FROM upstreams WHERE slug='rest'`).Scan(&ciphertext); err != nil || len(ciphertext) == 0 || strings.Contains(string(ciphertext), "rest-secret") {
		t.Fatal("not encrypted", err)
	}
	request := map[string]any{"operation": "lookup", "arguments": map[string]any{}}
	a.expect(200, "POST", "/api/v1/upstreams/rest/rest/preview", request)
	a.expect(422, "POST", "/api/v1/upstreams/rest/refresh-tools", nil)
	a.expect(422, "POST", "/api/v1/upstreams/rest/connection-test", nil)
	a.expect(412, "POST", "/api/v1/upstreams/rest/rest/test", request)
	if calls.Load() != 0 {
		t.Fatal("implicit execution")
	}
	code, _, result := a.do("POST", "/api/v1/upstreams/rest/rest/test", request, map[string]string{"If-Match": h.Get("ETag")})
	if code != 200 || result["isError"] == true || calls.Load() != 1 {
		t.Fatalf("test: %d %v", code, result)
	}
	_, reader := a.expect(201, "POST", "/api/v1/admin-tokens", map[string]any{"slug": "rest-reader", "scope": "read"})
	for _, action := range []string{"preview", "test"} {
		if code, _, _ := a.do("POST", "/api/v1/upstreams/rest/rest/"+action, request, map[string]string{"Authorization": "Bearer " + reader["token"].(string), "If-Match": h.Get("ETag")}); code != 403 {
			t.Fatalf("reader allowed %s: %d", action, code)
		}
	}
	a.expect(422, "PATCH", "/api/v1/upstreams/rest", map[string]any{"kind": "mcp"})
	a.expect(422, "PATCH", "/api/v1/upstreams/rest", map[string]any{"headers": map[string]string{"x-vmcp-agent-id": "spoof"}})
	_, doc := a.expect(200, "GET", "/api/v1/export", nil)
	raw, _ = json.Marshal(doc)
	if strings.Contains(string(raw), "rest-secret") || strings.Contains(string(raw), "operation-secret") {
		t.Fatal("export leaked secrets")
	}
	a.expect(200, "POST", "/api/v1/apply?dryRun=true", doc)
	a.expect(200, "POST", "/api/v1/apply", doc)
	if calls.Load() != 1 {
		t.Fatal("apply executed REST")
	}
	h, _ = a.expect(200, "GET", "/api/v1/upstreams/rest", nil)
	code, _, result = a.do("POST", "/api/v1/upstreams/rest/rest/test", request, map[string]string{"If-Match": h.Get("ETag")})
	if code != 200 || result["isError"] == true || calls.Load() != 2 {
		t.Fatalf("roundtrip: %d %v", code, result)
	}
	a.expect(200, "PATCH", "/api/v1/upstreams/rest", map[string]any{"enabled": false})
	if code, _, _ := a.do("POST", "/api/v1/upstreams/rest/rest/test", request, map[string]string{"If-Match": h.Get("ETag")}); code != 403 {
		t.Fatalf("disabled execution: %d", code)
	}
}

func TestNestedResponseProjectionConfigurationRoundtrip(t *testing.T) {
	a := setup(t)
	paths := [][]string{{"order", "items", "productId"}, {"order", "items", "quantity"}}
	operation := map[string]any{"name": "lookup", "method": "GET", "path": "/orders", "enabled": true, "responseSchema": json.RawMessage(`{"type":"object","properties":{"order":{"type":"object","properties":{"items":{"type":"array","items":{"type":"object","properties":{"productId":{"type":"string"},"quantity":{"type":"integer"},"cost":{"type":"number"}}}}}}}}`), "responseFieldPaths": paths}
	payload := map[string]any{"slug": "nested-rest", "name": "Nested REST", "kind": "rest", "url": "https://example.com", "rest": map[string]any{"auth": map[string]any{"type": "none"}, "timeoutSeconds": 30, "operations": []any{operation}}}
	a.expect(201, "POST", "/api/v1/upstreams", payload)
	check := func() {
		t.Helper()
		_, out := a.expect(200, "GET", "/api/v1/upstreams/nested-rest", nil)
		raw, _ := json.Marshal(out)
		var decoded struct {
			REST struct {
				Operations []struct{ ResponseFieldPaths [][]string }
			}
		}
		if err := json.Unmarshal(raw, &decoded); err != nil {
			t.Fatal(err)
		}
		got, _ := json.Marshal(decoded.REST.Operations[0].ResponseFieldPaths)
		want, _ := json.Marshal(paths)
		if string(got) != string(want) {
			t.Fatalf("projection changed: %s != %s", got, want)
		}
	}
	check()
	paths = [][]string{{"order", "items", "productId"}}
	operation["responseFieldPaths"] = paths
	a.expect(200, "PATCH", "/api/v1/upstreams/nested-rest", map[string]any{"rest": payload["rest"]})
	check()
	_, doc := a.expect(200, "GET", "/api/v1/export", nil)
	a.expect(200, "POST", "/api/v1/apply?dryRun=true", doc)
	a.expect(200, "POST", "/api/v1/apply", doc)
	check()
	operation["responseFieldPaths"] = [][]string{{"order", "missing"}}
	a.expect(422, "PATCH", "/api/v1/upstreams/nested-rest", map[string]any{"rest": payload["rest"]})
	check()
}

func TestRESTParameterValueConfigurationRoundtrip(t *testing.T) {
	a := setup(t)
	parameters := []any{
		map[string]any{"name": "type", "in": "query", "exposeToMcp": false, "defaultValue": 1, "schema": map[string]any{"type": "integer", "enum": []int{1, 2}}},
		map[string]any{"name": "content", "in": "query", "required": true, "defaultValue": "all", "schema": map[string]any{"type": "string"}},
	}
	rest := map[string]any{"auth": map[string]any{"type": "none"}, "timeoutSeconds": 30, "operations": []any{map[string]any{"name": "sample", "method": "GET", "path": "/sample", "enabled": true, "parameters": parameters}}}
	a.expect(201, "POST", "/api/v1/upstreams", map[string]any{"slug": "defaults", "name": "Defaults", "kind": "rest", "url": "https://example.com", "rest": rest})
	check := func(want float64) {
		t.Helper()
		_, out := a.expect(200, "GET", "/api/v1/upstreams/defaults", nil)
		ops := out["rest"].(map[string]any)["operations"].([]any)
		ps := ops[0].(map[string]any)["parameters"].([]any)
		p := ps[0].(map[string]any)
		if p["exposeToMcp"] != false || p["defaultValue"] != want {
			t.Fatalf("lost fixed value: %v", p)
		}
	}
	check(1)
	parameters[0].(map[string]any)["defaultValue"] = 2
	a.expect(200, "PATCH", "/api/v1/upstreams/defaults", map[string]any{"rest": rest})
	_, doc := a.expect(200, "GET", "/api/v1/export", nil)
	a.expect(200, "POST", "/api/v1/apply?dryRun=true", doc)
	a.expect(200, "POST", "/api/v1/apply", doc)
	check(2)
	parameters[0].(map[string]any)["defaultValue"] = 3
	a.expect(422, "PATCH", "/api/v1/upstreams/defaults", map[string]any{"rest": rest})
	check(2)
}

func TestRelativePathSnapshotMatchesManagementPreview(t *testing.T) {
	a := setup(t)
	cfg := restapi.Config{Auth: restapi.Auth{Type: "none"}, TimeoutSeconds: 30, Operations: []restapi.Operation{{Name: "browse", Enabled: true, Method: "GET", Path: "/browse/{path}", Parameters: []restapi.Parameter{{Name: "path", In: "path", PathMode: "relative", Schema: json.RawMessage(`{"type":"string"}`)}}}}}
	const base = "https://example.com"
	a.expect(201, "POST", "/api/v1/upstreams", map[string]any{"slug": "bitbucket", "name": "Bitbucket", "kind": "rest", "url": base, "rest": cfg})
	_, saved := a.expect(200, "GET", "/api/v1/upstreams/bitbucket", nil)
	raw, _ := json.Marshal(saved["rest"])
	var read restapi.Config
	if err := json.Unmarshal(raw, &read); err != nil || read.Operations[0].Parameters[0].PathMode != "relative" {
		t.Fatal(string(raw), err)
	}
	snap, err := store.LoadSnapshot(context.Background(), a.pool, a.control.Store.Box)
	if err != nil {
		t.Fatal(err)
	}
	var runtime *restapi.Compiled
	for _, u := range snap.Upstreams {
		if u.Slug == "bitbucket" {
			runtime = u.REST
		}
	}
	if runtime == nil {
		t.Fatal("bitbucket REST upstream missing from snapshot")
	}
	for _, path := range []string{"", "src/main"} {
		args := map[string]string{}
		if path != "" {
			args["path"] = path
		}
		_, preview := a.expect(200, "POST", "/api/v1/upstreams/bitbucket/rest/preview", map[string]any{"operation": "browse", "arguments": args})
		raw, _ := json.Marshal(args)
		request, err := runtime.Build(base, "browse", raw, nil, restapi.Identity{})
		if err != nil || request.URL.String() != preview["url"] {
			t.Fatal(preview, request, err)
		}
	}
}

func TestTextResponseManagementRoundTrip(t *testing.T) {
	t.Setenv("VMCP_EGRESS_ALLOW_CIDRS", "127.0.0.0/8")
	a := setup(t)
	received := make(chan string, 3)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		accept := r.Header.Get("Accept")
		received <- accept
		if accept == "application/json" {
			w.Write([]byte(`{"id":1}`))
			return
		}
		w.Write([]byte("report\n"))
	}))
	defer srv.Close()
	cfg := restapi.Config{Auth: restapi.Auth{Type: "none"}, TimeoutSeconds: 5, AllowInsecureHTTP: true, Operations: []restapi.Operation{{Name: "report", Enabled: true, Method: "GET", Path: "/report", ResponseSchema: json.RawMessage(`{"type":"object"}`)}}}
	const up = "/api/v1/upstreams"
	a.expect(201, "POST", up, map[string]any{"slug": "reports", "name": "reports", "kind": "rest", "url": srv.URL, "rest": cfg})
	args := map[string]any{"operation": "report", "arguments": map[string]any{}}
	before, _ := a.expect(200, "GET", up+"/reports", nil)
	cfg.Operations[0].ResponseSchema = nil
	cfg.Operations[0].ResponseFormat = "text"
	_, original := a.expect(200, "POST", up+"/reports/rest/preview", args)
	if original["headers"].(map[string]any)["Accept"] != "application/json" || len(received) != 0 {
		t.Fatal(original)
	}
	code, _, result := a.do("POST", up+"/reports/rest/test", args, map[string]string{"If-Match": before.Get("ETag")})
	if code != 200 || result["isError"] == true || len(received) != 1 || <-received != "application/json" {
		t.Fatal(code, result)
	}
	a.expect(200, "PATCH", up+"/reports", map[string]any{"rest": cfg})
	headers, saved := a.expect(200, "GET", up+"/reports", nil)
	raw, _ := json.Marshal(saved["rest"])
	var read restapi.Config
	if err := json.Unmarshal(raw, &read); err != nil || read.Operations[0].ResponseFormat != "text" {
		t.Fatal(read, err)
	}

	_, preview := a.expect(200, "POST", up+"/reports/rest/preview", args)
	if preview["headers"].(map[string]any)["Accept"] != "text/plain" {
		t.Fatal(preview)
	}
	code, _, result = a.do("POST", up+"/reports/rest/test", args, map[string]string{"If-Match": headers.Get("ETag")})
	if len(received) != 1 || <-received != "text/plain" || code != 200 || result["structuredContent"].(map[string]any)["result"] != "report\n" {
		t.Fatal(result)
	}
	snap, err := store.LoadSnapshot(context.Background(), a.pool, a.control.Store.Box)
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range snap.Upstreams {
		if u.Slug == "reports" {
			req, err := u.REST.Build(srv.URL, "report", json.RawMessage(`{}`), nil, restapi.Identity{})
			if err != nil || req.Header.Get("Accept") != "text/plain" {
				t.Fatal(req, err)
			}
			return
		}
	}
	t.Fatal("saved upstream missing from snapshot")
}
