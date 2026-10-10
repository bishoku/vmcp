// Package policy decides which of a virtual server's tools agents may see and call.
package policy

import (
	"encoding/json"
	"fmt"
	"sort"
)

// Modes a tool can be in.
const (
	Allow    = "allow"
	Deny     = "deny"
	ReadOnly = "read_only" // allowed only if the upstream declares readOnlyHint: true
)

// Config is stored per virtual server. The zero value allows everything.
type Config struct {
	// Default applies to tools without an entry in Tools. Empty means allow.
	Default string `json:"default"`
	// Tools overrides Default, keyed by exposed tool name.
	Tools map[string]string `json:"tools"`
}

func validMode(m string) bool { return m == Allow || m == Deny || m == ReadOnly }

// Normalize makes equivalent configs compare equal.
func Normalize(c Config) Config {
	if c.Default == "" {
		c.Default = Allow
	}
	if len(c.Tools) == 0 {
		c.Tools = nil
	}
	return c
}

// Validate returns problems keyed by JSON pointer relative to the policy object.
// exposed is the set of exposed tool names of the virtual server.
func Validate(c Config, exposed map[string]bool) map[string]string {
	errs := map[string]string{}
	if c.Default != "" && !validMode(c.Default) {
		errs["/default"] = "must be allow, deny or read_only"
	}
	for name, m := range c.Tools {
		p := "/tools/" + escape(name)
		switch {
		case !validMode(m):
			errs[p] = "must be allow, deny or read_only"
		case !exposed[name]:
			errs[p] = fmt.Sprintf("no exposed tool named %q", name)
		}
	}
	return errs
}

func escape(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '~':
			out = append(out, '~', '0')
		case '/':
			out = append(out, '~', '1')
		default:
			out = append(out, s[i])
		}
	}
	return string(out)
}

// Decide returns whether the tool may be used and, if not, why.
// annotations is the upstream's raw tool annotations (may be empty).
func (c Config) Decide(exposed string, annotations json.RawMessage) (ok bool, reason string) {
	mode := c.Default
	if m, found := c.Tools[exposed]; found {
		mode = m
	}
	switch mode {
	case Deny:
		return false, "policy: deny"
	case ReadOnly:
		if !readOnlyHint(annotations) {
			return false, "policy: read_only"
		}
	}
	return true, ""
}

func readOnlyHint(a json.RawMessage) bool {
	if len(a) == 0 {
		return false
	}
	var v struct {
		ReadOnlyHint *bool `json:"readOnlyHint"`
	}
	return json.Unmarshal(a, &v) == nil && v.ReadOnlyHint != nil && *v.ReadOnlyHint
}

// SortedTools lists overridden tool names deterministically.
func (c Config) SortedTools() []string {
	names := make([]string, 0, len(c.Tools))
	for n := range c.Tools {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
