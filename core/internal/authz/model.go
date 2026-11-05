// Package authz implements control-plane permission decisions. It does not grant
// runtime MCP access or replace resource ownership checks in compatibility mode.
package authz

import "slices"

type Scope struct {
	Kind      string `json:"kind"`
	ID        string `json:"id,omitempty"`
	Epoch     string `json:"epoch,omitempty"`
	OwnerTeam string `json:"ownerTeam,omitempty"`
}

type Role struct {
	ID          string   `json:"id"`
	Slug        string   `json:"slug"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Builtin     bool     `json:"builtin"`
	Version     int64    `json:"version"`
	Permissions []string `json:"permissions"`
}

// Principal identifiers are issuer+subject for OIDC, or immutable token IDs.
type Principal struct {
	Kind   string `json:"kind"`
	Issuer string `json:"issuer,omitempty"`
	ID     string `json:"id"`
}
type Assignment struct {
	ID            string    `json:"id"`
	RoleID        string    `json:"roleId"`
	Principal     Principal `json:"principal"`
	MappingID     string    `json:"mappingId,omitempty"`
	Scope         Scope     `json:"scope"`
	CaptureScopes []string  `json:"captureScopes"`
	Enabled       bool      `json:"enabled"`
	Version       int64     `json:"version"`
}

type Resource struct {
	Tenant    string
	Kind      string
	ID        string
	Project   string
	OwnerTeam string
	Epoch     string
}

type Request struct {
	Principal  Principal
	Permission string
	Resource   Resource
	// Audit marks historical access: project and resource scopes must pin their
	// recorded ownership domain, rather than following current ownership.
	Audit        bool
	CaptureScope string
}

type Decision struct {
	Allowed      bool
	Reason       string
	AssignmentID string
}
type Policy struct {
	Tenant      string
	Revision    int64
	Roles       []Role
	Assignments []Assignment
	Mappings    []Mapping
}

func (s Scope) valid() bool {
	switch s.Kind {
	case "tenant", "unmanaged":
		return s.ID == "" && s.Epoch == "" && s.OwnerTeam == ""
	case "team", "project", "upstream", "virtual_server":
		return s.ID != ""
	}
	return false
}

func (s Scope) matches(r Resource, historical bool) bool {
	if !s.valid() {
		return false
	}
	if !historical && s.Epoch != "" && s.Epoch != r.Epoch {
		return false
	}
	switch s.Kind {
	case "tenant":
		return true
	case "unmanaged":
		return (r.Kind == "upstream" || r.Kind == "virtual_server") && r.OwnerTeam == ""
	case "team":
		return r.OwnerTeam == s.ID
	case "project":
		return r.Project == s.ID && (!historical || s.OwnerTeam == r.OwnerTeam)
	default:
		return s.Kind == r.Kind && s.ID == r.ID && (!historical || s.Epoch != "" && s.Epoch == r.Epoch)
	}
}

// Decide is default-deny. Permission, scope and capture restrictions must all
// come from one assignment. A stale/unloaded policy (revision 0) cannot allow.
func (p Policy) Decide(q Request) Decision {
	deny := Decision{Reason: "no_matching_assignment"}
	if p.Validate() != nil {
		return Decision{Reason: "invalid_policy"}
	}
	if p.Tenant == "" || p.Tenant != q.Resource.Tenant || p.Revision <= 0 {
		return Decision{Reason: "policy_unavailable"}
	}
	spec, ok := Lookup(q.Permission)
	if !ok {
		return Decision{Reason: "unknown_permission"}
	}
	if !q.Principal.valid() {
		return Decision{Reason: "invalid_principal"}
	}
	for _, a := range p.Assignments {
		if !a.Enabled || a.MappingID != "" || a.Principal != q.Principal || !slices.Contains(spec.Scopes, a.Scope.Kind) || !a.Scope.matches(q.Resource, q.Audit) {
			continue
		}
		if spec.Capture && (q.CaptureScope == "" || !ValidCaptureScope(q.CaptureScope) || !slices.Contains(a.CaptureScopes, q.CaptureScope)) {
			continue
		}
		for _, role := range p.Roles {
			if role.ID == a.RoleID && slices.Contains(role.Permissions, q.Permission) {
				return Decision{Allowed: true, Reason: "assignment", AssignmentID: a.ID}
			}
		}
	}
	return deny
}
