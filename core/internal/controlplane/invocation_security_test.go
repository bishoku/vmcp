package controlplane_test

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestInvocationSecurityManagement(t *testing.T) {
	a := setup(t)
	_, initial := a.expect(200, "GET", "/api/v1/invocation-security", nil)
	doc := map[string]any{"revision": initial["revision"], "services": map[string]any{"demo": map[string]any{"url": "https://approval.example.test/check", "approvalOrigin": "https://approval.example.test", "token": "service-secret-must-never-be-returned"}}, "upstreams": map[string]any{}}
	_, saved := a.expect(200, "PUT", "/api/v1/invocation-security", doc)
	raw, _ := json.Marshal(saved)
	if strings.Contains(string(raw), "service-secret-must-never-be-returned") {
		t.Fatal("service token leaked")
	}
	a.expect(412, "PUT", "/api/v1/invocation-security", doc)
	_, saved = a.expect(200, "PUT", "/api/v1/invocation-security", saved)
	services := saved["services"].(map[string]any)
	services["demo"].(map[string]any)["url"] = "https://other.example.test/check"
	a.expect(422, "PUT", "/api/v1/invocation-security", saved)
	token, id := payloadReader(t, a)
	headers := map[string]string{"Authorization": "Bearer " + token}
	code, _, _ := a.do("GET", "/api/v1/invocation-security", nil, headers)
	if code != 403 {
		t.Fatalf("unassigned read %d", code)
	}
	configurationGrant(t, a, id, []string{"invocation_security.read"}, "tenant", nil, nil)
	code, _, _ = a.do("GET", "/api/v1/invocation-security", nil, headers)
	if code != 200 {
		t.Fatalf("authorized read %d", code)
	}
	code, _, _ = a.do("PUT", "/api/v1/invocation-security", saved, headers)
	if code != 403 {
		t.Fatalf("read granted write %d", code)
	}
}

func TestInvocationIdentityConfigurationAndPortableExport(t *testing.T) {
	a := setup(t)
	a.expect(201, "POST", "/api/v1/upstreams", map[string]any{"slug": "customers", "name": "Customers", "url": a.fixture.URL, "enabled": true, "callerContext": map[string]any{"enabled": true, "metaKey": "app/customer", "fields": []any{map[string]any{"name": "customerId", "required": true}, map[string]any{"name": "sessionId", "required": true}}}})
	_, scopes := a.expect(200, "GET", "/api/v1/access/scopes?kind=upstream", nil)
	var id string
	for _, item := range scopes["items"].([]any) {
		u := item.(map[string]any)
		if u["slug"] == "customers" {
			id = u["id"].(string)
		}
	}
	if id == "" {
		t.Fatal("upstream missing")
	}
	_, doc := a.expect(200, "GET", "/api/v1/invocation-security", nil)
	doc["services"] = map[string]any{"session": map[string]any{"url": "https://session.test/check", "approvalOrigin": "https://session.test", "token": "external-session-secret"}}
	identity := map[string]any{"source": "caller_context", "service": "session", "fields": []string{"customerId", "sessionId"}}
	doc["upstreams"] = map[string]any{id: map[string]any{"identity": identity, "tools": map[string]any{"transfer": map[string]any{"stepUp": map[string]any{"acceptedAcrValues": []string{"2"}, "maxAgeSeconds": 60}}}}}
	_, saved := a.expect(200, "PUT", "/api/v1/invocation-security", doc)
	a.expect(409, "DELETE", "/api/v1/upstreams/customers?force=true", nil)
	identity["fields"] = []string{"notConfigured"}
	doc["revision"] = saved["revision"]
	a.expect(422, "PUT", "/api/v1/invocation-security", doc)
	_, exported := a.expect(200, "GET", "/api/v1/export", nil)
	portable := exported["invocationSecurity"].(map[string]any)
	if portable["upstreams"].(map[string]any)["customers"] == nil {
		t.Fatal("stable ID was not exported as slug")
	}
	raw, _ := json.Marshal(portable)
	if strings.Contains(string(raw), "external-session-secret") {
		t.Fatal("export leaked secret")
	}
	// Omitted exported credentials retain the existing service credential on apply.
	a.expect(200, "POST", "/api/v1/apply?refreshTools=false", map[string]any{"apiVersion": "vmcp/v1", "invocationSecurity": portable})
	_, roundtrip := a.expect(200, "GET", "/api/v1/invocation-security", nil)
	if roundtrip["upstreams"].(map[string]any)[id] == nil {
		t.Fatal("portable source did not resolve to upstream ID")
	}
}
