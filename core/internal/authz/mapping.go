package authz

import "strings"

type Condition struct {
	Path     string `json:"path"`
	Operator string `json:"operator"`
	Value    string `json:"value"`
}
type Mapping struct {
	ID            string      `json:"id"`
	Issuer        string      `json:"issuer"`
	PrincipalKind string      `json:"principalKind"`
	Conditions    []Condition `json:"conditions"`
	Enabled       bool        `json:"enabled"`
	Version       int64       `json:"version"`
}

// Match accepts verified claims only. An exact claim key precedes dotted paths,
// matching the existing runtime claim convention. Empty mappings never match.
func (m Mapping) Match(who Principal, claims map[string]any) bool {
	if !m.Enabled || m.ID == "" || m.Issuer == "" || m.Issuer != who.Issuer || m.PrincipalKind != who.Kind || who.Kind != "oidc" || len(m.Conditions) == 0 {
		return false
	}
	for _, c := range m.Conditions {
		if c.Path == "" || c.Value == "" {
			return false
		}
		v, ok := claims[c.Path]
		if !ok {
			var cur any = claims
			for _, part := range strings.Split(c.Path, ".") {
				obj, yes := cur.(map[string]any)
				if !yes {
					cur = nil
					break
				}
				cur = obj[part]
			}
			v = cur
		}
		match := false
		switch c.Operator {
		case "equals":
			s, ok := v.(string)
			match = ok && s == c.Value
		case "array_contains":
			switch xs := v.(type) {
			case []any:
				for _, x := range xs {
					if s, ok := x.(string); ok && s == c.Value {
						match = true
					}
				}
			case []string:
				for _, x := range xs {
					if x == c.Value {
						match = true
					}
				}
			}
		case "scope_contains":
			if s, ok := v.(string); ok {
				for _, x := range strings.Fields(s) {
					if x == c.Value {
						match = true
					}
				}
			}
		}
		if !match {
			return false
		}
	}
	return true
}

// ResolveMappings returns a copy of the policy with matching mapping assignments
// bound to the authenticated subject. Claims must already satisfy freshness and
// issuer/signature validation in the caller; this function never authenticates.
func (p Policy) ResolveMappings(who Principal, claims map[string]any) Policy {
	out := p
	out.Assignments = append([]Assignment(nil), p.Assignments...)
	for i, a := range out.Assignments {
		if a.MappingID == "" {
			continue
		}
		for _, m := range p.Mappings {
			if m.ID == a.MappingID && m.Match(who, claims) {
				out.Assignments[i].Principal = who
				out.Assignments[i].MappingID = ""
				break
			}
		}
	}
	return out
}
