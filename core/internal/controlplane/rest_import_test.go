package controlplane_test

import (
	"context"
	"testing"
)

func TestRESTImportPreviewIsAuthorizedAndDoesNotMutate(t *testing.T) {
	a := setup(t)
	a.expect(201, "POST", "/api/v1/upstreams", a.upstream())
	_, before := a.expect(200, "GET", "/api/v1/info", nil)
	document := `{"openapi":"3.1.0","info":{"title":"Import"},"servers":[{"url":"http://169.254.169.254/latest/meta-data"}],"paths":{"/ok":{"get":{"operationId":"ok","responses":{"204":{}}}},"/external":{"get":{"responses":{"200":{"content":{"application/json":{"schema":{"$ref":"http://127.0.0.1/private"}}}}}}}}}`
	_, result := a.expect(200, "POST", "/api/v1/rest-imports", map[string]string{"document": document})
	operations := result["operations"].([]any)
	if len(operations) != 2 {
		t.Fatal(result)
	}
	if operations[0].(map[string]any)["operation"] != nil || operations[1].(map[string]any)["operation"] == nil {
		t.Fatal("external reference was not isolated", result)
	}
	_, after := a.expect(200, "GET", "/api/v1/info", nil)
	if before["configRevision"] != after["configRevision"] {
		t.Fatal("preview mutated revision")
	}
	var count int
	if err := a.pool.QueryRow(context.Background(), `SELECT count(*) FROM upstreams`).Scan(&count); err != nil || count != 1 {
		t.Fatal("import created an upstream", count, err)
	}
	a.expect(422, "POST", "/api/v1/rest-imports", map[string]string{"document": "openapi: 9.0.0"})
	a.expect(422, "POST", "/api/v1/rest-imports", map[string]string{"url": "https://example.com/spec"})
	_, reader := a.expect(201, "POST", "/api/v1/admin-tokens", map[string]any{"slug": "reader", "scope": "read"})
	if code, _, _ := a.do("POST", "/api/v1/rest-imports", map[string]string{"document": document}, map[string]string{"Authorization": "Bearer " + reader["token"].(string)}); code != 403 {
		t.Fatal("viewer can import", code)
	}
	// Selected operations use the ordinary REST save path; import does not bypass it.
	op := operations[1].(map[string]any)["operation"]
	payload := map[string]any{"slug": "imported", "name": "Imported", "kind": "rest", "url": "https://example.com", "rest": map[string]any{"auth": map[string]string{"type": "none"}, "operations": []any{op}}}
	_, saved := a.expect(201, "POST", "/api/v1/upstreams", payload)
	if saved["toolCount"] != float64(1) {
		t.Fatal(saved)
	}
	payload["slug"] = "insecure"
	payload["url"] = "http://example.com"
	a.expect(422, "POST", "/api/v1/upstreams", payload)
}
