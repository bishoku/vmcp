package controlplane_test

import "testing"

func TestRESTSchemaChecksAreAuthenticatedAndTransient(t *testing.T) {
	a := setup(t)
	const path = "/api/v1/rest-schema-checks"
	body := map[string]any{"checks": []any{
		map[string]any{"schema": false},
		map[string]any{"schema": false, "sample": nil},
	}}
	if status, _, _ := a.do("POST", path, body, map[string]string{"Authorization": ""}); status != 401 {
		t.Fatalf("unauthenticated validation returned %d", status)
	}
	_, before := a.expect(200, "GET", "/api/v1/info", nil)
	_, result := a.expect(200, "POST", path, body)
	issues := result["issues"].([]any)
	if len(issues) != 2 || len(issues[0].([]any)) != 0 || len(issues[1].([]any)) == 0 {
		t.Fatalf("unexpected validation results: %v", issues)
	}
	a.expect(422, "POST", path, map[string]any{"checks": []any{}})
	_, after := a.expect(200, "GET", "/api/v1/info", nil)
	if before["configRevision"] != after["configRevision"] {
		t.Fatal("validation mutated configuration")
	}
}
