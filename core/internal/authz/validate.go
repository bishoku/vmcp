package authz

import "fmt"

func (p Policy) Validate() error {
	if p.Tenant == "" || p.Revision <= 0 {
		return fmt.Errorf("invalid authorization state")
	}
	roles := map[string]bool{}
	for _, r := range p.Roles {
		if r.ID == "" || roles[r.ID] {
			return fmt.Errorf("duplicate or missing role ID")
		}
		roles[r.ID] = true
		for _, id := range r.Permissions {
			if _, ok := Lookup(id); !ok {
				return fmt.Errorf("unknown permission %q", id)
			}
		}
	}
	mappings := map[string]bool{}
	for _, m := range p.Mappings {
		if m.ID == "" || mappings[m.ID] || m.Issuer == "" || m.PrincipalKind != "oidc" || len(m.Conditions) == 0 {
			return fmt.Errorf("invalid mapping")
		}
		mappings[m.ID] = true
		for _, c := range m.Conditions {
			if c.Path == "" || c.Value == "" {
				return fmt.Errorf("empty mapping condition")
			}
			switch c.Operator {
			case "equals", "array_contains", "scope_contains":
			default:
				return fmt.Errorf("unknown mapping operator")
			}
		}
	}
	assignments := map[string]bool{}
	for _, a := range p.Assignments {
		if a.ID == "" || assignments[a.ID] || !roles[a.RoleID] || !a.Scope.valid() {
			return fmt.Errorf("invalid assignment")
		}
		assignments[a.ID] = true
		if a.MappingID != "" {
			if !mappings[a.MappingID] || a.Principal != (Principal{}) {
				return fmt.Errorf("invalid mapping assignment")
			}
		} else if !a.Principal.valid() {
			return fmt.Errorf("invalid principal")
		}
		for _, scope := range a.CaptureScopes {
			if !ValidCaptureScope(scope) {
				return fmt.Errorf("invalid capture scope")
			}
		}
	}
	return nil
}
func (p Principal) valid() bool {
	if p.ID == "" {
		return false
	}
	return p.Kind == "oidc" && p.Issuer != "" || p.Kind == "control_token" && p.Issuer == ""
}
