package controlplane

import (
	"go/ast"
	"go/parser"
	"go/token"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestConfigurationRegisteredRouteInventory(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	tree, err := parser.ParseFile(token.NewFileSet(), filepath.Join(filepath.Dir(file), "server.go"), nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	ast.Inspect(tree, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || len(call.Args) < 1 {
			return true
		}
		name, ok := call.Fun.(*ast.Ident)
		if !ok || name.Name != "api" {
			return true
		}
		value, ok := call.Args[0].(*ast.BasicLit)
		if !ok {
			return true
		}
		pattern, err := strconv.Unquote(value.Value)
		if err != nil {
			t.Fatal(err)
		}
		method, path, _ := strings.Cut(pattern, " ")
		r := httptest.NewRequest(method, strings.ReplaceAll(strings.ReplaceAll(path, "{slug}", "sample"), "{action}", "preview"), nil)
		r.Pattern = pattern
		r.SetPathValue("slug", "sample")
		r.SetPathValue("action", "preview")
		if strings.Contains(path, "/mock/") {
			r.SetPathValue("action", "sync")
		}
		if authorizationManagementRoute(r) || payloadRoute(r) || auditListRoute(r) {
			return true
		}
		if _, known := configurationOperation(r); !known {
			t.Errorf("registered configuration route has no policy: %s", pattern)
		}
		count++
		return true
	})
	if count < 45 {
		t.Fatalf("route inventory unexpectedly incomplete: %d", count)
	}
	for _, family := range []string{"teams", "projects", "tool-grants"} {
		for _, method := range []string{"GET", "POST"} {
			r := httptest.NewRequest(method, "/api/v1/"+family, nil)
			r.Pattern = method + " /api/v1/" + family
			if _, ok := configurationOperation(r); !ok {
				t.Fatal(r.Pattern)
			}
		}
	}
	r := httptest.NewRequest("GET", "/api/v1/future-config", nil)
	r.Pattern = "GET /api/v1/future-config"
	if _, ok := configurationOperation(r); ok {
		t.Fatal("unknown routes must not inherit generic GET rights")
	}
}
func TestConfigurationBodyRejectsAmbiguousAliases(t *testing.T) {
	for _, raw := range []string{`{"URL":"one","url":"two"}`, `{"project":"one","project":"two"}`, `{} {}`, `null`} {
		if _, err := configurationBody([]byte(raw)); err == nil {
			t.Fatal("ambiguous body accepted", raw)
		}
	}
	body, err := configurationBody([]byte(`{"Name":"Name","REST":{"operations":[{"bodySchema":{"properties":{"CaseSensitive":{},"casesensitive":{}}}}]}}`))
	if err != nil || body["name"] != "Name" {
		t.Fatal("nested REST schema must remain case-sensitive", body, err)
	}
}
