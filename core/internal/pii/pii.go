// Package pii masks personal data in tool arguments and results.
//
// Detectors are RE2 regular expressions (linear time, so operator-supplied patterns
// cannot cause catastrophic backtracking) followed by checksum validation where the
// data type has one, which keeps false positives down.
package pii

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// Config is the per-virtual-server masking policy.
type Config struct {
	Enabled bool `json:"enabled"`
	// Detectors names built-in detectors; empty means all of them.
	Detectors []string `json:"detectors,omitempty"`
	// CustomRules add operator-defined patterns.
	CustomRules []Rule `json:"customRules,omitempty"`
	// MaskArguments also masks tool arguments before they reach the upstream.
	MaskArguments bool `json:"maskArguments,omitempty"`
}

// Rule is a custom detector. Matches are replaced with [REDACTED:<name>].
type Rule struct {
	Name    string `json:"name"`
	Pattern string `json:"pattern"`
}

const (
	maxRules      = 20
	maxPatternLen = 500
)

type detector struct {
	re       *regexp.Regexp
	validate func(string) bool // nil = every regex match counts
}

var ipv4RE = regexp.MustCompile(`^\d{1,3}(\.\d{1,3}){3}$`)

// builtins are applied in this order: specific, checksum-validated types first so
// the looser phone pattern cannot claim their digits.
var builtinOrder = []string{"iban", "credit_card", "tckn", "us_ssn", "email", "phone"}

var builtins = map[string]detector{
	"email":       {regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`), nil},
	"credit_card": {regexp.MustCompile(`\b(?:\d[ -]?){12,18}\d\b`), validCard},
	"iban":        {regexp.MustCompile(`\b[A-Z]{2}\d{2}(?: ?[A-Z0-9]{4}){2,7}(?: ?[A-Z0-9]{1,3})?\b`), validIBAN},
	"tckn":        {regexp.MustCompile(`\b[1-9]\d{10}\b`), validTCKN},
	"us_ssn":      {regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`), validSSN},
	"phone": {regexp.MustCompile(`\+\d{10,14}\b|(?:\+\d{1,3}[\s.-]?)?(?:\(\d{2,4}\)|\d{2,4})[\s.-]\d{3}[\s.-]?\d{2}[\s.-]?\d{2,4}`),
		func(s string) bool { return !ipv4RE.MatchString(s) }},
}

// DetectorNames lists the built-in detectors.
func DetectorNames() []string { return slices.Clone(builtinOrder) }

// Validate returns a message per problem, keyed by JSON pointer relative to the config.
func Validate(c Config) map[string]string {
	errs := map[string]string{}
	for i, n := range c.Detectors {
		if _, ok := builtins[n]; !ok {
			errs[fmt.Sprintf("/detectors/%d", i)] = fmt.Sprintf("unknown detector %q (known: %s)", n, strings.Join(builtinOrder, ", "))
		}
	}
	if len(c.CustomRules) > maxRules {
		errs["/customRules"] = fmt.Sprintf("at most %d rules", maxRules)
	}
	seen := map[string]bool{}
	for i, r := range c.CustomRules {
		p := fmt.Sprintf("/customRules/%d", i)
		if !ruleNameRE.MatchString(r.Name) {
			errs[p+"/name"] = "must be 1-40 characters: letters, digits, underscore or hyphen"
		} else if seen[r.Name] {
			errs[p+"/name"] = "duplicate rule name"
		}
		seen[r.Name] = true
		switch {
		case r.Pattern == "" || len(r.Pattern) > maxPatternLen:
			errs[p+"/pattern"] = fmt.Sprintf("must be 1-%d characters", maxPatternLen)
		default:
			re, err := regexp.Compile(r.Pattern)
			if err != nil {
				errs[p+"/pattern"] = "invalid regular expression: " + err.Error()
			} else if re.MatchString("") {
				errs[p+"/pattern"] = "must not match the empty string"
			}
		}
	}
	return errs
}

var ruleNameRE = regexp.MustCompile(`^[A-Za-z0-9_-]{1,40}$`)

type rule struct {
	name string
	det  detector
}

// Masker applies a compiled Config.
type Masker struct {
	rules         []rule
	MaskArguments bool
}

// Compile validates and compiles c. A disabled config yields a nil Masker.
func Compile(c Config) (*Masker, error) {
	if !c.Enabled {
		return nil, nil
	}
	if errs := Validate(c); len(errs) > 0 {
		for k, v := range errs {
			return nil, fmt.Errorf("pii config %s: %s", k, v)
		}
	}
	names := c.Detectors
	if len(names) == 0 {
		names = builtinOrder
	}
	m := &Masker{MaskArguments: c.MaskArguments}
	for _, n := range builtinOrder { // fixed order regardless of how the operator listed them
		if slices.Contains(names, n) {
			m.rules = append(m.rules, rule{strings.ToUpper(n), builtins[n]})
		}
	}
	for _, r := range c.CustomRules {
		m.rules = append(m.rules, rule{r.Name, detector{re: regexp.MustCompile(r.Pattern)}})
	}
	return m, nil
}

// MaskString replaces every detected value and returns the number of replacements.
func (m *Masker) MaskString(s string) (string, int) {
	n := 0
	for _, r := range m.rules {
		s = r.det.re.ReplaceAllStringFunc(s, func(match string) string {
			if r.det.validate != nil && !r.det.validate(match) {
				return match
			}
			n++
			return "[REDACTED:" + r.name + "]"
		})
	}
	return s, n
}

// MaskJSON masks every string value in a JSON document (object keys are left alone).
// Binary payloads of image/audio content blocks are skipped.
func (m *Masker) MaskJSON(raw json.RawMessage) (json.RawMessage, int, error) {
	if len(raw) == 0 {
		return raw, 0, nil
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, 0, err
	}
	n := 0
	v = m.walk(v, &n)
	if n == 0 {
		return raw, 0, nil // keep the original bytes when nothing changed
	}
	out, err := json.Marshal(v)
	return out, n, err
}

func (m *Masker) walk(v any, n *int) any {
	switch x := v.(type) {
	case string:
		s, c := m.MaskString(x)
		*n += c
		return s
	case []any:
		for i := range x {
			x[i] = m.walk(x[i], n)
		}
		return x
	case map[string]any:
		binary := false
		if t, _ := x["type"].(string); t == "image" || t == "audio" {
			binary = true
		}
		for k, e := range x {
			if binary && (k == "data" || k == "mimeType") {
				continue
			}
			x[k] = m.walk(e, n)
		}
		return x
	}
	return v
}

func digitsOf(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func validCard(s string) bool {
	d := digitsOf(s)
	if len(d) < 13 || len(d) > 19 {
		return false
	}
	sum, alt := 0, false
	for i := len(d) - 1; i >= 0; i-- {
		n := int(d[i] - '0')
		if alt {
			if n *= 2; n > 9 {
				n -= 9
			}
		}
		sum += n
		alt = !alt
	}
	return sum%10 == 0
}

func validIBAN(s string) bool {
	s = strings.ReplaceAll(s, " ", "")
	if len(s) < 15 || len(s) > 34 {
		return false
	}
	s = s[4:] + s[:4]
	rem := 0
	for _, r := range s {
		var part int
		switch {
		case r >= '0' && r <= '9':
			part = int(r - '0')
			rem = (rem*10 + part) % 97
		case r >= 'A' && r <= 'Z':
			part = int(r-'A') + 10
			rem = (rem*100 + part) % 97
		default:
			return false
		}
	}
	return rem == 1
}

func validTCKN(s string) bool {
	if len(s) != 11 || s[0] == '0' {
		return false
	}
	d := make([]int, 11)
	for i := range d {
		d[i] = int(s[i] - '0')
	}
	odd := d[0] + d[2] + d[4] + d[6] + d[8]
	even := d[1] + d[3] + d[5] + d[7]
	if ((odd*7-even)%10+10)%10 != d[9] {
		return false
	}
	sum := 0
	for _, x := range d[:10] {
		sum += x
	}
	return sum%10 == d[10]
}

func validSSN(s string) bool {
	area, group, serial := s[:3], s[4:6], s[7:]
	return area != "000" && area != "666" && area[0] != '9' && group != "00" && serial != "0000"
}
