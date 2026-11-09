package controlplane

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/bishoku/vmcp/core/internal/apierror"
	"github.com/bishoku/vmcp/core/internal/store"
)

// Explicit inventory. Unknown routes/actions never acquire a generic update or
// read permission simply because their method or path resembles a known route.
func configurationOperation(r *http.Request) (store.ConfigurationOperation, bool) {
	op := store.ConfigurationOperation{Slug: r.PathValue("slug")}
	method, path, _ := strings.Cut(r.Pattern, " ")
	path = strings.TrimPrefix(path, "/api/v1/")
	parts := strings.Split(path, "/")
	family := parts[0]
	switch family {
	case "configuration-access":
		op.Family = "self"
		return op, method == "POST"
	case "me":
		op.Family = "self"
		return op, method == "GET"
	case "info", "openapi.json", "openapi.yaml":
		op.Family = "platform"
		op.Action = "info.read"
		return op, method == "GET"
	case "upstreams", "virtual-servers", "agent-keys", "identity-bindings", "admin-tokens", "teams", "projects", "tool-grants":
		names := map[string]string{"virtual-servers": "virtual_servers", "agent-keys": "agent_keys", "identity-bindings": "identity_bindings", "admin-tokens": "control_tokens", "tool-grants": "tool_grants"}
		op.Family = family
		if name := names[family]; name != "" {
			op.Family = name
		}
		verbs := map[string]string{"GET": "read", "POST": "create", "PUT": "replace", "PATCH": "update", "DELETE": "delete"}
		op.Verb = verbs[method]
		if (family == "upstreams" || family == "virtual-servers") && len(parts) >= 3 && parts[2] == "audit-settings" {
			op.Family = "audit_local"
			op.ResourceKind = "upstream"
			if family == "virtual-servers" {
				op.ResourceKind = "virtual_server"
			}
			if len(parts) == 3 && method == "GET" {
				op.Verb = "read"
				return op, true
			}
			if len(parts) == 3 && method == "PUT" {
				op.Verb = "update"
				return op, true
			}
			if len(parts) == 4 && parts[3] == "preview" && method == "POST" {
				op.Action = "preview"
				return op, true
			}
			return op, false
		}
		if len(parts) > 2 {
			op.Action = parts[2]
			if method != "GET" {
				op.Verb = "action"
			}
			if family == "upstreams" {
				switch parts[2] {
				case "invocation-security":
					if len(parts) != 3 || (method != "GET" && method != "PUT") {
						return op, false
					}
					op.Action = "invocation_security.read"
					if method == "PUT" {
						op.Action = "invocation_security.manage"
					}
				case "tools", "catalog":
					op.Action = "catalog.read"
				case "refresh-tools":
					op.Action = "catalog.refresh"
				case "connection-test":
					op.Action = "test"
				case "rest":
					switch r.PathValue("action") {
					case "preview":
						op.Action = "rest.preview"
					case "test":
						op.Action = "rest.execute"
					default:
						return op, false
					}
				case "mock":
					action := r.PathValue("action")
					if len(parts) > 3 && parts[3] == "status" {
						action = "status"
					}
					switch action {
					case "status":
						op.Action = "mock.read"
					case "sync":
						op.Action = "mock.sync"
					default:
						return op, false
					}
				default:
					return op, false
				}
			} else if !((family == "agent-keys" || family == "admin-tokens") && (op.Action == "rotate" || op.Action == "revoke")) {
				return op, false
			}
		} else if len(parts) == 2 && parts[1] != "{slug}" {
			if family == "identity-bindings" && parts[1] == "evaluate" {
				op.Action = "evaluate"
				op.Verb = "action"
			} else if family == "tool-grants" && parts[1] == "bulk" {
				op.Action = "bulk"
				op.Verb = "action"
			} else {
				return op, false
			}
		}
		return op, op.Verb != ""
	case "tool-catalog":
		op.Family = "tool_catalog"
		op.Verb = "read"
		return op, method == "GET"
	case "connection-tests":
		op.Family = "connections"
		op.Action = "test_unsaved"
		return op, method == "POST"
	case "rest-imports":
		op.Family = "rest"
		op.Action = "import"
		return op, method == "POST"
	case "mock-provider":
		op.Family = "mock.provider"
		op.Verb = "read"
		return op, method == "GET"
	case "revocations":
		if len(parts) != 2 {
			return op, false
		}
		if parts[1] == "jwts" {
			op.Family = "runtime_jwts"
		} else if parts[1] == "sessions" {
			op.Family = "sessions"
		} else {
			return op, false
		}
		op.Action = "revoke"
		return op, method == "POST"
	case "invocation-security":
		op.Family = "invocation_security"
		if method == "GET" {
			op.Action = "read"
		} else if method == "PUT" {
			op.Action = "manage"
		} else {
			return op, false
		}
		return op, len(parts) == 1 || (len(parts) == 2 && parts[1] == "shared-services")
	case "admission-settings":
		op.Family = "admission"
		if method == "GET" {
			op.Action = "read"
		} else if method == "PUT" {
			op.Action = "update"
		} else {
			return op, false
		}
		return op, len(parts) == 1
	case "audit-settings":
		op.Family = "audit"
		if method == "GET" {
			op.Action = "settings.read"
		} else if method == "PUT" {
			op.Action = "settings.update"
		} else {
			return op, false
		}
		return op, true
	case "audit-preview":
		op.Family = "audit"
		op.Action = "redaction.preview"
		return op, method == "POST"
	case "audit-status":
		op.Family = "audit"
		op.Action = "status.read"
		return op, method == "GET"
	case "omni":
		op.Family = "omni"
		if len(parts) == 1 {
			op.Action = "status.read"
			return op, method == "GET"
		}
		if len(parts) == 2 && (parts[1] == "preview" || parts[1] == "reindex") {
			op.Action = parts[1]
			return op, method == "POST"
		}
	case "export":
		op.Family = "config"
		op.Action = "export"
		return op, method == "GET"
	case "apply":
		op.Family = "config"
		op.Action = "apply"
		op.Prune = queryBool(r, "prune", false)
		op.DryRun = queryBool(r, "dryRun", false)
		op.Refresh = queryBool(r, "refreshTools", true)
		return op, method == "POST"
	}
	return op, false
}
func configurationBody(raw []byte) (map[string]any, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return map[string]any{}, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return nil, fmt.Errorf("configuration body must be an object")
	}
	body := map[string]any{}
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		name := strings.ToLower(key.(string))
		if _, exists := body[name]; exists {
			return nil, fmt.Errorf("duplicate or ambiguous configuration field")
		}
		var value any
		if err := decoder.Decode(&value); err != nil {
			return nil, err
		}
		body[name] = value
	}
	if _, err := decoder.Token(); err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, fmt.Errorf("unexpected data after configuration body")
	}
	return body, nil
}
func (s *Server) prepareConfiguration(r *http.Request, p *principal) (*http.Request, bool, error) {
	if authorizationManagementRoute(r) || payloadRoute(r) || auditListRoute(r) {
		return r, false, nil
	}
	policy, err := store.LoadAuthorization(r.Context(), s.Store.Pool)
	if err != nil {
		return r, false, apierror.New(503, "authorization_unavailable", "Authorization unavailable", "Configuration policy could not be verified")
	}
	// Explicit platform administrator grants do not upgrade custom roles by slug.
	policy = (payloadPolicy{Policy: policy}).effective(p)
	op, known := configurationOperation(r)
	op.GlobalAdmin = p.Role == "admin"
	var decision error
	if !known {
		decision = apierror.Forbidden("configuration route has no registered permission")
	}
	if decision == nil && r.Method != "GET" && r.Method != "HEAD" && r.Method != "DELETE" {
		raw, readErr := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes+1))
		r.Body = io.NopCloser(bytes.NewReader(raw))
		if readErr != nil || len(raw) > maxBodyBytes {
			decision = apierror.Validation([]apierror.FieldError{{Message: "configuration body exceeds limit"}})
		} else {
			op.Body, err = configurationBody(raw)
			if err != nil {
				decision = apierror.Validation([]apierror.FieldError{{Message: "invalid or ambiguous configuration JSON"}})
			}
		}
	}
	if decision == nil {
		decision = s.Store.Reader().CheckConfiguration(r.Context(), policy, payloadPrincipal(p), op)
	}
	if decision != nil {
		return r, false, decision
	}
	return r.WithContext(store.WithConfigurationPolicy(r.Context(), policy, payloadPrincipal(p), op)), true, nil
}
