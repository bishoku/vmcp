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

func TestUpstreamInvocationSecurityDelegation(t *testing.T) {
	a, owner, _ := controlAuditFixture(t)
	a.expect(201, "POST", "/api/v1/upstreams", map[string]any{"slug": "support", "name": "Support", "url": a.fixture.URL, "ownerTeam": "after"})
	if _, err := a.pool.Exec(t.Context(), `INSERT INTO upstream_tools(upstream_id,name,input_schema)
SELECT id,'transfer','{"type":"object"}'::jsonb FROM upstreams WHERE slug IN ('crm','support')`); err != nil {
		t.Fatal(err)
	}
	token, id := payloadReader(t, a)
	configurationGrant(t, a, id, []string{"upstreams.invocation_security.read", "upstreams.invocation_security.manage"}, "team", owner, nil)
	headers := map[string]string{"Authorization": "Bearer " + token}
	request := func(want int, method, path string, body any) map[string]any {
		t.Helper()
		code, _, out := a.do(method, path, body, headers)
		if code != want {
			t.Fatalf("%s %s: got %d, want %d: %v", method, path, code, want, out)
		}
		return out
	}

	const ownPath = "/api/v1/upstreams/crm/invocation-security"
	const otherPath = "/api/v1/upstreams/support/invocation-security"
	initial := request(200, "GET", ownPath, nil)
	_, other := a.expect(200, "GET", otherPath, nil)
	request(403, "GET", otherPath, nil)
	request(403, "PUT", otherPath, map[string]any{"upstreamId": other["upstreamId"], "revision": other["revision"], "policy": map[string]any{}})
	request(403, "GET", "/api/v1/invocation-security", nil)

	newService := func(host, token string) map[string]any {
		return map[string]any{"url": "https://" + host + "/check", "approvalOrigin": "https://" + host, "token": token}
	}
	shared := newService("shared.example", "shared-secret")
	_, legacy := a.expect(200, "GET", "/api/v1/invocation-security", nil)
	legacy["services"] = map[string]any{"shared": shared}
	legacy["upstreams"] = map[string]any{}
	a.expect(200, "PUT", "/api/v1/invocation-security", legacy)
	if names := request(200, "GET", ownPath, nil)["sharedServices"].([]any); len(names) != 1 || names[0] != "shared" {
		t.Fatal("shared service unavailable", names)
	}

	rule := map[string]any{"approvalServices": []string{"local"}, "retry": "once", "readOnly": false}
	policy := map[string]any{
		"identity": map[string]any{"source": "oauth_subject"}, "approvalServices": []string{"shared"},
		"services": map[string]any{"local": newService("crm.example", "crm-local-secret")},
		"tools":    map[string]any{"typo": rule},
	}
	doc := map[string]any{"upstreamId": initial["upstreamId"], "revision": initial["revision"], "policy": policy}
	request(422, "PUT", ownPath, doc)
	policy["tools"] = map[string]any{"transfer": rule}
	saved := request(200, "PUT", ownPath, doc)
	savedRule := saved["policy"].(map[string]any)["tools"].(map[string]any)["transfer"].(map[string]any)
	if savedRule["retry"] != "once" || savedRule["readOnly"] != false {
		t.Fatal("execution policy lost", savedRule)
	}
	if raw, _ := json.Marshal(saved); strings.Contains(string(raw), "crm-local-secret") || strings.Contains(string(raw), "shared-secret") {
		t.Fatal("scoped response disclosed a credential")
	}
	request(412, "PUT", ownPath, doc)

	const sharedPath = "/api/v1/invocation-security/shared-services"
	request(403, "GET", sharedPath, nil)
	_, sharedView := a.expect(200, "GET", sharedPath, nil)
	if sharedView["upstreams"] != nil || len(sharedView["inUse"].([]any)) != 1 || sharedView["inUse"].([]any)[0] != "shared" {
		t.Fatal("shared endpoint returned the wrong service scope", sharedView)
	}
	a.expect(422, "PUT", sharedPath, map[string]any{"revision": sharedView["revision"], "services": map[string]any{}})
	a.expect(200, "PUT", sharedPath, map[string]any{"revision": sharedView["revision"], "services": sharedView["services"]})

	// A revision read before the first team's save can still update another upstream.
	otherUpdate := map[string]any{"upstreamId": other["upstreamId"], "revision": other["revision"], "policy": map[string]any{
		"identity": map[string]any{"source": "oauth_subject"}, "tools": map[string]any{"transfer": rule},
	}}
	a.expect(422, "PUT", otherPath, otherUpdate)
	otherUpdate["policy"].(map[string]any)["services"] = map[string]any{"local": newService("support.example", "support-local-secret")}
	a.expect(200, "PUT", otherPath, otherUpdate)

	_, exported := a.expect(200, "GET", "/api/v1/export", nil)
	portable := exported["invocationSecurity"]
	if raw, _ := json.Marshal(portable); strings.Contains(string(raw), "local-secret") || strings.Contains(string(raw), "shared-secret") {
		t.Fatal("GitOps export disclosed a credential")
	}
	a.expect(200, "POST", "/api/v1/apply?refreshTools=false", map[string]any{"apiVersion": "vmcp/v1", "invocationSecurity": portable})

	staged := saved["policy"].(map[string]any)
	delete(staged, "tools")
	doc = map[string]any{"upstreamId": saved["upstreamId"], "revision": saved["revision"], "policy": staged}
	current := request(200, "PUT", ownPath, doc)
	if next := current["policy"].(map[string]any); next["tools"] != nil || next["identity"] == nil {
		t.Fatal("removing a tool rule changed the upstream default", next)
	}
	if _, err := a.pool.Exec(t.Context(), `DELETE FROM authorization_role_permissions
WHERE permission_id='upstreams.invocation_security.manage' AND role_id=(SELECT id FROM authorization_roles WHERE slug='config-test')`); err != nil {
		t.Fatal(err)
	}
	request(200, "GET", ownPath, nil)
	request(403, "PUT", ownPath, doc)
}

func TestInvocationServiceAuthenticationRoundTrip(t *testing.T) {
	a := setup(t)
	_, doc := a.expect(200, "GET", "/api/v1/invocation-security", nil)
	doc["services"] = map[string]any{
		"oauth":   map[string]any{"url": "https://approval.test/check", "approvalOrigin": "https://approval.test", "authType": "oauth_client_credentials", "timeoutSeconds": 5, "oauth": map[string]any{"tokenUrl": "https://idp.test/token", "clientId": "client", "clientSecret": "oauth-private-value"}},
		"headers": map[string]any{"url": "https://approval.test/check", "approvalOrigin": "https://approval.test", "authType": "headers", "headers": map[string]string{"X-Key": "header-private-value"}},
		"public":  map[string]any{"url": "https://approval.test/check", "approvalOrigin": "https://approval.test", "authType": "none"},
	}
	_, saved := a.expect(200, "PUT", "/api/v1/invocation-security", doc)
	assertRedacted := func(value any) {
		t.Helper()
		raw, _ := json.Marshal(value)
		if strings.Contains(string(raw), "oauth-private-value") || strings.Contains(string(raw), "header-private-value") {
			t.Fatal("credential leaked")
		}
	}
	assertRedacted(saved)
	services := saved["services"].(map[string]any)
	if services["oauth"].(map[string]any)["oauth"].(map[string]any)["hasClientSecret"] != true || services["headers"].(map[string]any)["hasHeaders"].(map[string]any)["X-Key"] != true {
		t.Fatal("missing credential status")
	}
	// A read-modify-write retains both types of redacted credentials.
	_, saved = a.expect(200, "PUT", "/api/v1/invocation-security", saved)
	assertRedacted(saved)
	_, exported := a.expect(200, "GET", "/api/v1/export", nil)
	assertRedacted(exported)
	a.expect(200, "POST", "/api/v1/apply?refreshTools=false", map[string]any{"apiVersion": "vmcp/v1", "invocationSecurity": exported["invocationSecurity"]})
	_, saved = a.expect(200, "GET", "/api/v1/invocation-security", nil)
	services = saved["services"].(map[string]any)
	services["oauth"].(map[string]any)["oauth"].(map[string]any)["tokenUrl"] = "https://other.test/token"
	a.expect(422, "PUT", "/api/v1/invocation-security", saved)
	services["oauth"].(map[string]any)["oauth"].(map[string]any)["clientSecret"] = "replacement"
	_, saved = a.expect(200, "PUT", "/api/v1/invocation-security", saved)
	services = saved["services"].(map[string]any)
	services["headers"].(map[string]any)["url"] = "https://other.test/check"
	a.expect(422, "PUT", "/api/v1/invocation-security", saved)
	// Explicit none removes credentials rather than inheriting them from the old mode.
	services["headers"] = map[string]any{"url": "https://other.test/check", "approvalOrigin": "https://other.test", "authType": "none"}
	a.expect(200, "PUT", "/api/v1/invocation-security", saved)
}
