package controlplane_test

import "testing"

func TestIdentityBindings(t *testing.T) {
	a := setup(t)
	a.expect(201, "POST", "/api/v1/upstreams", a.upstream())
	a.expect(200, "POST", "/api/v1/upstreams/fx/refresh-tools", nil)
	a.expect(201, "POST", "/api/v1/virtual-servers", map[string]any{
		"slug": "vs", "name": "VS", "tools": []map[string]string{{"upstream": "fx", "tool": "echo"}}})

	cond := []map[string]any{{"claim": "azp", "op": "equals", "value": "billing-bot"}}
	good := map[string]any{"slug": "billing", "conditions": cond, "virtualServers": []string{"vs"},
		"ipAllowlist": []string{"10.0.0.0/8"}, "rateLimitPerMinute": 60}

	// Validation.
	for name, mut := range map[string]map[string]any{
		"no conditions":  {"conditions": []any{}},
		"bad op":         {"conditions": []map[string]any{{"claim": "a", "op": "regex", "value": "x"}}},
		"unknown server": {"virtualServers": []string{"nope"}},
		"bad cidr":       {"ipAllowlist": []string{"not-an-ip"}},
		"negative limit": {"rateLimitPerMinute": -1},
	} {
		body := map[string]any{}
		for k, v := range good {
			body[k] = v
		}
		for k, v := range mut {
			body[k] = v
		}
		if code, _, out := a.do("POST", "/api/v1/identity-bindings", body, nil); code != 422 && code != 400 {
			t.Fatalf("%s: code=%d %v", name, code, out)
		}
	}

	a.expect(201, "POST", "/api/v1/identity-bindings", good)
	a.expect(409, "POST", "/api/v1/identity-bindings", good)
	h, out := a.expect(200, "GET", "/api/v1/identity-bindings/billing", nil)
	if out["rateLimitPerMinute"].(float64) != 60 || h.Get("ETag") == "" {
		t.Fatalf("get: %v %v", out, h)
	}
	_, list := a.expect(200, "GET", "/api/v1/identity-bindings", nil)
	if items, _ := list["items"].([]any); len(items) != 1 {
		t.Fatalf("list: %v", list)
	}

	// PATCH: null clears the limit; stale ETag is rejected.
	if code, _, _ := a.do("PATCH", "/api/v1/identity-bindings/billing", map[string]any{"enabled": false}, map[string]string{"If-Match": `"999"`}); code != 412 {
		t.Fatalf("stale etag: %d", code)
	}
	_, out = a.expect(200, "PATCH", "/api/v1/identity-bindings/billing", map[string]any{"rateLimitPerMinute": nil, "enabled": false})
	if out["rateLimitPerMinute"] != nil || out["enabled"] != false {
		t.Fatalf("patch: %v", out)
	}

	// Evaluate dry-run (enabled again).
	a.expect(200, "PATCH", "/api/v1/identity-bindings/billing", map[string]any{"enabled": true})
	ev := func(claims map[string]any, extra map[string]any) map[string]any {
		body := map[string]any{"claims": claims, "virtualServer": "vs"}
		for k, v := range extra {
			body[k] = v
		}
		_, o := a.expect(200, "POST", "/api/v1/identity-bindings/evaluate", body)
		return o
	}
	if o := ev(map[string]any{"azp": "billing-bot"}, map[string]any{"clientIp": "10.1.2.3"}); !allowed(o) {
		t.Fatalf("allowed: %v", o)
	}
	if o := ev(map[string]any{"azp": "x"}, nil); allowed(o) {
		t.Fatalf("no match must not be allowed: %v", o)
	}
	if o := ev(map[string]any{"azp": "billing-bot"}, map[string]any{"clientIp": "192.168.0.1"}); allowed(o) {
		t.Fatalf("ip outside allowlist: %v", o)
	}

	// Export/apply: bindings round-trip; absent = untouched; present-but-empty + prune removes.
	_, doc := a.expect(200, "GET", "/api/v1/export", nil)
	if b, _ := doc["bindings"].([]any); len(b) != 1 {
		t.Fatalf("export bindings: %v", doc["bindings"])
	}
	a.expect(200, "POST", "/api/v1/apply?dryRun=true", doc)
	delete(doc, "bindings")
	a.expect(200, "POST", "/api/v1/apply?prune=true", doc)
	a.expect(200, "GET", "/api/v1/identity-bindings/billing", nil)
	doc["bindings"] = []any{}
	a.expect(200, "POST", "/api/v1/apply?prune=true", doc)
	a.expect(404, "GET", "/api/v1/identity-bindings/billing", nil)

	a.expect(201, "POST", "/api/v1/identity-bindings", good)
	a.expect(204, "DELETE", "/api/v1/identity-bindings/billing", nil)
}

func allowed(o map[string]any) bool {
	d, _ := o["decision"].(map[string]any)
	ok, _ := d["allowed"].(bool)
	return ok
}
