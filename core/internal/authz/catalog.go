package authz

import "strings"

type Permission struct {
	ID      string   `json:"id"`
	Scopes  []string `json:"scopes"`
	Capture bool     `json:"captureRestricted"`
}

func buildCatalog() []Permission {
	var out []Permission
	add := func(ns, actions string, scopes []string) {
		for _, action := range strings.Fields(actions) {
			out = append(out, Permission{ID: ns + "." + action, Scopes: append([]string(nil), scopes...)})
		}
	}
	tenant := []string{"tenant"}
	upstream := []string{"tenant", "team", "upstream", "unmanaged"}
	server := []string{"tenant", "team", "project", "virtual_server", "unmanaged"}
	add("platform", "info.read", tenant)
	add("teams", "read manage", []string{"tenant", "team"})
	add("projects", "read manage", []string{"tenant", "team", "project", "unmanaged"})
	add("ownership", "transfer", append(append([]string{}, upstream...), "project", "virtual_server"))
	add("upstreams", "read create update delete connection.update activation.update credentials.update tls.update identity_forwarding.update rest.manage catalog.read catalog.refresh test rest.preview rest.execute mock.read mock.sync", upstream)
	add("virtual_servers", "read create update delete publication.update policy.update client_features.update", server)
	add("agent_keys", "read create update delete access.update rotate revoke constraints.update audit_correlation.update", server)
	add("identity_bindings", "read create update delete rules.update access.update constraints.update evaluate", server)
	add("control_tokens", "read create update delete access.update rotate revoke constraints.update", tenant)
	add("tool_catalog", "read", server)
	add("tool_grants", "read request approve reject revoke", []string{"tenant", "team", "project", "upstream", "virtual_server", "unmanaged"})
	add("connections", "test_unsaved", tenant)
	add("rest", "import", tenant)
	add("mock.provider", "read", tenant)
	add("runtime_jwts", "revoke", tenant)
	add("sessions", "revoke", tenant)
	add("omni", "status.read status.details.read preview reindex", tenant)
	add("config", "export validate apply prune", tenant)
	for _, family := range []string{"roles", "assignments", "mappings"} {
		add("access."+family, "read manage", tenant)
	}
	add("access", "evaluate", tenant)
	audit := []string{"tenant", "team", "project", "upstream", "virtual_server", "unmanaged"}
	add("audit", "events.read identities.read details.read settings.read redaction.local.update redaction.preview", audit)
	add("audit", "status.read durability.update retention.update capture.update redaction.global.update export_policy.update diagnostics.update diagnostics.denied.read legacy.events.read legacy.payloads.read legacy.payloads.unclassified.read", tenant)
	add("audit.payloads", "request.read response.read", audit)
	for i := range out {
		if strings.HasPrefix(out[i].ID, "audit.payloads.") {
			out[i].Capture = true
		}
	}
	return out
}

var permissionCatalog = buildCatalog()
var permissionIndex = func() map[string]Permission {
	out := make(map[string]Permission, len(permissionCatalog))
	for _, p := range permissionCatalog {
		out[p.ID] = p
	}
	return out
}()

// Catalog returns a copy so callers cannot change authorization rules.
func Catalog() []Permission {
	out := make([]Permission, len(permissionCatalog))
	for i, p := range permissionCatalog {
		out[i] = p
		out[i].Scopes = append([]string(nil), p.Scopes...)
	}
	return out
}
func Lookup(id string) (Permission, bool) {
	p, ok := permissionIndex[id]
	if ok {
		p.Scopes = append([]string(nil), p.Scopes...)
	}
	return p, ok
}
func ValidCaptureScope(s string) bool {
	switch s {
	case "virtual", "upstream", "control", "reads", "system", "omni", "omni_diagnostics", "omni_embedding", "omni_rerank":
		return true
	}
	return false
}
