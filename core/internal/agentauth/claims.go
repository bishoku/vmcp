// Package agentauth authenticates agents that present a JWT issued by an external identity
// provider and maps its claims to identity bindings.
package agentauth

import (
	"strings"

	"github.com/bishoku/vmcp/core/internal/config"
)

// Lookup reads a claim. An exact top-level key wins; otherwise a dotted path such as
// "realm_access.roles" is walked through nested objects. A missing claim yields nil.
func Lookup(claims map[string]any, path string) any {
	if v, ok := claims[path]; ok {
		return v
	}
	if !strings.Contains(path, ".") {
		return nil
	}
	cur := any(claims)
	for _, part := range strings.Split(path, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = m[part]
	}
	return cur
}

// List turns a claim value into strings: a JSON array of strings, or a single string (split on
// whitespace when splitSpace is set, as for the space-separated "scope" claim).
func List(v any, splitSpace bool) []string {
	switch x := v.(type) {
	case []any:
		out := make([]string, 0, len(x))
		for _, e := range x {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case string:
		if splitSpace {
			return strings.Fields(x)
		}
		return []string{x}
	}
	return nil
}

// Binding condition operators.
const (
	OpEquals   = "equals"   // scalar claim equals the value
	OpContains = "contains" // list claim (or space-separated string) contains the value
	OpExists   = "exists"   // claim is present and not empty
)

// Match reports whether every condition holds for the claims. No conditions never match.
func Match(conds []config.Condition, claims map[string]any) bool {
	if len(conds) == 0 {
		return false
	}
	for _, c := range conds {
		v := Lookup(claims, c.Claim)
		switch c.Op {
		case OpEquals:
			s, ok := v.(string)
			if !ok || s != c.Value {
				return false
			}
		case OpContains:
			found := false
			for _, e := range List(v, true) {
				if e == c.Value {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		case OpExists:
			switch x := v.(type) {
			case nil:
				return false
			case string:
				if x == "" {
					return false
				}
			case []any:
				if len(x) == 0 {
					return false
				}
			}
		default:
			return false
		}
	}
	return true
}

// Outcome says why Resolve did or did not allow a request.
type Outcome int

const (
	Allowed      Outcome = iota
	NoMatch              // no binding matches the token's claims
	ServerDenied         // bindings match, but none lists the virtual server
	IPDenied             // a binding lists the server, but the client address is not allowed
)

// Resolve returns the first binding (the snapshot orders them by slug) that matches the claims,
// lists the virtual server and passes ipOK (nil = any address).
func Resolve(bindings []config.Binding, claims map[string]any, vs string, ipOK func(config.Binding) bool) (*config.Binding, Outcome) {
	matched, served := false, false
	for i := range bindings {
		b := &bindings[i]
		if !Match(b.Conditions, claims) {
			continue
		}
		matched = true
		if !b.Servers[vs] {
			continue
		}
		served = true
		if ipOK != nil && !ipOK(*b) {
			continue
		}
		return b, Allowed
	}
	switch {
	case !matched:
		return nil, NoMatch
	case !served:
		return nil, ServerDenied
	}
	return nil, IPDenied
}
