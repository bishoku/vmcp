package controlplane_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUpstreamOAuthAPI(t *testing.T) {
	a := setup(t)
	// The fixture upstream accepts the bearer "up-secret": the token endpoint hands exactly that out.
	idp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		user, pass, basic := r.BasicAuth()
		secret := r.PostForm.Get("client_secret")
		if basic {
			secret = pass
			_ = user
		}
		if secret != "s3cret" {
			http.Error(w, `{"error":"invalid_client"}`, 401)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"up-secret","token_type":"Bearer","expires_in":3600}`))
	}))
	t.Cleanup(idp.Close)

	body := func() map[string]any {
		return map[string]any{"slug": "oa", "name": "OA", "url": a.fixture.URL, "transport": "streamable_http",
			"auth": map[string]any{"type": "oauth_client_credentials", "tokenUrl": idp.URL, "clientId": "gw", "clientSecret": "s3cret",
				"scopes": []string{"mcp.read"}, "audience": "fixture"}}
	}

	// Validation.
	for name, mut := range map[string]func(m map[string]any){
		"missing secret": func(m map[string]any) { delete(m["auth"].(map[string]any), "clientSecret") },
		"bad token url":  func(m map[string]any) { m["auth"].(map[string]any)["tokenUrl"] = "ftp://x" },
		"bad scope":      func(m map[string]any) { m["auth"].(map[string]any)["scopes"] = []string{"a b"} },
		"static Authorization header": func(m map[string]any) {
			m["headers"] = map[string]string{"authorization": "Bearer x"}
		},
	} {
		m := body()
		mut(m)
		if code, _, out := a.do("POST", "/api/v1/upstreams", m, nil); code != 422 {
			t.Fatalf("%s: code=%d %v", name, code, out)
		}
	}

	_, out := a.expect(201, "POST", "/api/v1/upstreams", body())
	auth := out["auth"].(map[string]any)
	if auth["type"] != "oauth_client_credentials" || auth["clientId"] != "gw" || auth["hasClientSecret"] != true {
		t.Fatalf("auth read model: %v", auth)
	}
	raw, _ := json.Marshal(out)
	if strings.Contains(string(raw), "s3cret") {
		t.Fatal("client secret returned by the API")
	}

	// The gateway can fetch tools through the token it obtains itself.
	_, res := a.expect(200, "POST", "/api/v1/upstreams/oa/refresh-tools", nil)
	if n, _ := res["toolCount"].(float64); n < 1 {
		t.Fatalf("refresh with oauth failed: %v", res)
	}

	// Export carries no secret; re-applying it keeps the stored one.
	_, doc := a.expect(200, "GET", "/api/v1/export", nil)
	raw, _ = json.Marshal(doc)
	if strings.Contains(string(raw), "s3cret") || strings.Contains(string(raw), "hasClientSecret") {
		t.Fatalf("export leaks or contains read-model fields: %s", raw)
	}
	a.expect(200, "POST", "/api/v1/apply?dryRun=true", doc)
	a.expect(200, "POST", "/api/v1/apply", doc)
	a.expect(200, "POST", "/api/v1/upstreams/oa/refresh-tools", nil)

	// PATCH merges: changing the scopes keeps the secret; a header conflict is refused.
	_, out = a.expect(200, "PATCH", "/api/v1/upstreams/oa", map[string]any{"auth": map[string]any{"scopes": []string{"a", "b"}}})
	if got := out["auth"].(map[string]any)["scopes"].([]any); len(got) != 2 {
		t.Fatalf("scopes: %v", got)
	}
	a.expect(200, "POST", "/api/v1/upstreams/oa/refresh-tools", nil)
	a.expect(422, "PATCH", "/api/v1/upstreams/oa", map[string]any{"headers": map[string]any{"Authorization": "Bearer x"}})

	// Wrong secret: the IdP error surfaces, the secret does not.
	a.expect(200, "PATCH", "/api/v1/upstreams/oa", map[string]any{"auth": map[string]any{"clientSecret": "wrong"}})
	code, _, fail := a.do("POST", "/api/v1/upstreams/oa/refresh-tools", nil, nil)
	if code != 502 {
		t.Fatalf("want 502 with a wrong secret, got %d %v", code, fail)
	}
	if b, _ := json.Marshal(fail); strings.Contains(string(b), "wrong") && strings.Contains(string(b), "client_secret=") {
		t.Fatalf("secret in error: %s", b)
	}

	// Connection test accepts an auth block.
	_, ct := a.expect(200, "POST", "/api/v1/connection-tests", map[string]any{"url": a.fixture.URL, "transport": "streamable_http",
		"auth": map[string]any{"type": "oauth_client_credentials", "tokenUrl": idp.URL, "clientId": "gw", "clientSecret": "s3cret"}})
	if ok, _ := ct["ok"].(bool); !ok {
		t.Fatalf("connection test with oauth: %v", ct)
	}

	// Removing OAuth.
	_, out = a.expect(200, "PATCH", "/api/v1/upstreams/oa", map[string]any{"auth": map[string]any{"type": "none"}})
	if out["auth"].(map[string]any)["type"] != "none" {
		t.Fatalf("auth not removed: %v", out["auth"])
	}
}
