package controlplane_test

import (
	"context"
	"testing"
)

func TestAuthorizationHasNoModeBypass(t *testing.T) {
	a := setup(t)
	token, id := payloadReader(t, a)
	headers := map[string]string{"Authorization": "Bearer " + token}
	request, response := payloadEvent(t, a, "request"), payloadEvent(t, a, "response")
	for _, selector := range []string{"legacy", "shadow", "enforced"} {
		headers["X-VMCP-Authorization-Mode"] = selector
		suffix := "?mode=" + selector + "&audit_payload_mode=" + selector
		code, _, out := a.do("GET", "/api/v1/upstreams"+suffix, nil, headers)
		if code != 403 {
			t.Fatal("unassigned configuration read bypass", selector, code, out)
		}
		code, _, out = a.do("GET", "/api/v1/audit-events/"+request+"/payload"+suffix, nil, headers)
		if code != 404 {
			t.Fatal("unassigned payload bypass", selector, code, out)
		}
	}
	configurationGrant(t, a, id, []string{"upstreams.read"}, "tenant", nil, nil)
	payloadGrant(t, a, id, []string{"audit.events.read", "audit.legacy.events.read", "audit.legacy.payloads.read", "audit.payloads.request.read"}, "tenant", nil, nil)
	for _, selector := range []string{"legacy", "shadow"} {
		suffix := "?mode=" + selector
		code, _, out := a.do("GET", "/api/v1/upstreams"+suffix, nil, headers)
		if code != 200 {
			t.Fatal("explicit read rejected", code, out)
		}
		code, _, out = a.do("GET", "/api/v1/audit-events/"+request+"/payload"+suffix, nil, headers)
		if code != 200 {
			t.Fatal("explicit request rejected", code, out)
		}
		code, _, out = a.do("GET", "/api/v1/audit-events/"+response+"/payload"+suffix, nil, headers)
		if code != 404 {
			t.Fatal("request grant became response grant", code, out)
		}
	}
	code, _, out := a.do("POST", "/api/v1/configuration-access", map[string]any{"mode": "legacy", "checks": []any{}}, headers)
	if code != 422 {
		t.Fatal("removed mode request accepted", code, out)
	}
	code, _, me := a.do("GET", "/api/v1/me", nil, headers)
	if code != 200 {
		t.Fatal(code, me)
	}
	for _, key := range []string{"mode", "auditMode", "configurationMode"} {
		if _, exists := me[key]; exists {
			t.Fatal("mode in me contract", key, me)
		}
	}
	_, status := a.expect(200, "GET", "/api/v1/access/status", nil)
	for _, key := range []string{"mode", "auditMode", "configurationMode", "configurationEnforcement", "configurationEnforcementReady", "legacyAdminCompatibility"} {
		if _, exists := status[key]; exists {
			t.Fatal("mode in status contract", key, status)
		}
	}
	code, _, _ = a.do("GET", "/api/v1/access/migration-status", nil, nil)
	if code != 404 {
		t.Fatal("removed activation-era endpoint remains", code)
	}
	var selectors int
	if err := a.pool.QueryRow(context.Background(), `SELECT count(*) FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='authorization_state' AND column_name IN ('mode','audit_payload_mode')`).Scan(&selectors); err != nil || selectors != 0 {
		t.Fatal(selectors, err)
	}
}
