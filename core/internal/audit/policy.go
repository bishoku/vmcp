package audit

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// Settings is immutable once attached to an operation. Local rules add protection;
// they never turn off global rules. Empty mode preserves the deployment default.
type Settings struct {
	ExportPayloads  map[string]bool    `json:"exportPayloads"`
	Mode            string             `json:"mode"`
	RetentionDays   int                `json:"retentionDays"`
	MaxPayloadBytes int                `json:"maxPayloadBytes"`
	Scopes          map[string]Capture `json:"scopes"`
	Rules           []Rule             `json:"rules"`
	VirtualServers  map[string][]Rule  `json:"virtualServers"`
	Upstreams       map[string][]Rule  `json:"upstreams"`
	Revision        int64              `json:"revision"`
}
type Capture struct {
	Request  bool `json:"request"`
	Response bool `json:"response"`
}
type Rule struct {
	Name    string `json:"name"`
	Pattern string `json:"pattern"`
}
type RuleMatch struct {
	Source string `json:"source"`
	Name   string `json:"name"`
	Count  int    `json:"count"`
}
type Preview struct {
	State         string          `json:"state"`
	Content       json.RawMessage `json:"content,omitempty"`
	OriginalBytes int             `json:"originalBytes"`
	StoredBytes   int             `json:"storedBytes"`
	Redactions    int             `json:"redactions"`
	Matches       []RuleMatch     `json:"matches"`
	Revision      int64           `json:"revision"`
}

func DefaultSettings() Settings {
	return Settings{RetentionDays: 7, MaxPayloadBytes: 64 << 10, Scopes: map[string]Capture{}, Rules: []Rule{}, VirtualServers: map[string][]Rule{}, Upstreams: map[string][]Rule{}}
}
func (s Settings) Normalize() Settings {
	if s.RetentionDays == 0 {
		s.RetentionDays = 7
	}
	if s.MaxPayloadBytes == 0 {
		s.MaxPayloadBytes = 64 << 10
	}
	if s.ExportPayloads == nil {
		s.ExportPayloads = map[string]bool{}
	}
	if s.Scopes == nil {
		s.Scopes = map[string]Capture{}
	}
	if s.Rules == nil {
		s.Rules = []Rule{}
	}
	if s.VirtualServers == nil {
		s.VirtualServers = map[string][]Rule{}
	}
	if s.Upstreams == nil {
		s.Upstreams = map[string][]Rule{}
	}
	return s
}
func (s Settings) Validate() error {
	if s.Mode != "" && s.Mode != "strict" && s.Mode != "relax" {
		return errors.New("mode must be strict or relax")
	}
	if s.RetentionDays < 1 || s.RetentionDays > 3650 {
		return errors.New("retentionDays must be 1-3650")
	}
	if s.MaxPayloadBytes < 256 || s.MaxPayloadBytes > 1<<20 {
		return errors.New("maxPayloadBytes must be 256-1048576")
	}
	for scope := range s.Scopes {
		switch scope {
		case "virtual", "upstream", "control", "system", "reads":
		default:
			return errors.New("unknown audit scope")
		}
	}
	if len(s.VirtualServers)+len(s.Upstreams) > 1000 {
		return errors.New("too many local policies")
	}
	for name := range s.ExportPayloads {
		if !regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`).MatchString(name) {
			return errors.New("invalid exporter consumer name")
		}
	}
	sets := [][]Rule{s.Rules}
	for _, r := range s.VirtualServers {
		sets = append(sets, r)
	}
	for _, r := range s.Upstreams {
		sets = append(sets, r)
	}
	for _, rules := range sets {
		if len(rules) > 20 {
			return errors.New("at most 20 rules per scope")
		}
		seen := map[string]bool{}
		for _, r := range rules {
			if !ruleName.MatchString(r.Name) || seen[r.Name] {
				return errors.New("rule names must be unique identifiers of 1-40 characters")
			}
			seen[r.Name] = true
			if len(r.Pattern) == 0 || len(r.Pattern) > 500 {
				return errors.New("regex must be 1-500 characters")
			}
			re, err := compiledRegex(r.Pattern)
			if err != nil {
				return fmt.Errorf("rule %s: invalid RE2 regex", r.Name)
			}
			if re.MatchString("") {
				return fmt.Errorf("rule %s must not match empty strings", r.Name)
			}
		}
	}
	return nil
}

var ruleName = regexp.MustCompile(`^[A-Za-z0-9_-]{1,40}$`)
var regexCache struct {
	sync.Mutex
	values map[string]*regexp.Regexp
}

func compiledRegex(pattern string) (*regexp.Regexp, error) {
	regexCache.Lock()
	defer regexCache.Unlock()
	if re := regexCache.values[pattern]; re != nil {
		return re, nil
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}
	if len(regexCache.values) >= 2048 || regexCache.values == nil {
		regexCache.values = map[string]*regexp.Regexp{}
	}
	regexCache.values[pattern] = re
	return re, nil
}
func secretKey(key string) bool {
	k := strings.ToLower(strings.NewReplacer("_", "", "-", "", ".", "").Replace(key))
	switch k {
	case "authorization", "proxyauthorization", "cookie", "setcookie", "password", "passwd", "secret", "clientsecret", "token", "accesstoken", "refreshtoken", "idtoken", "apikey", "masterkey", "clientkeypem", "privatekey", "headers", "tokenhash":
		return true
	}
	return false
}

// PreviewPayload is also the production masking path. Matches are found on the
// original value, and overlapping spans are merged before replacement.
func PreviewPayload(s Settings, scope, direction, vs, up string, raw json.RawMessage) (Preview, error) {
	s = s.Normalize()
	p := Preview{State: "disabled", OriginalBytes: len(raw), Matches: []RuleMatch{}, Revision: s.Revision}
	c := s.Scopes[scope]
	if direction != "request" && direction != "response" {
		return p, errors.New("direction must be request or response")
	}
	if direction == "request" && !c.Request || direction == "response" && !c.Response {
		return p, nil
	}
	if len(raw) == 0 {
		p.State = "empty"
		return p, nil
	}
	if len(raw) > 2<<20 {
		p.State = "omitted_size"
		return p, nil
	}
	type applied struct {
		source string
		rule   Rule
		re     *regexp.Regexp
	}
	var rules []applied
	for _, r := range []Rule{{"private-key", `(?s)-----BEGIN (?:[A-Z]+ )?PRIVATE KEY-----.*?-----END (?:[A-Z]+ )?PRIVATE KEY-----`}, {"bearer", `(?i)Bearer[ \t]+[A-Za-z0-9._~+/=-]+`}, {"vmcp-token", `vmcp_(?:admin_)?[A-Za-z0-9_-]{16,}`}} {
		re, _ := compiledRegex(r.Pattern)
		rules = append(rules, applied{"mandatory", r, re})
	}
	for _, set := range []struct {
		source string
		rules  []Rule
	}{{"global", s.Rules}, {"virtual:" + vs, s.VirtualServers[vs]}, {"upstream:" + up, s.Upstreams[up]}} {
		for _, r := range set.rules {
			re, err := compiledRegex(r.Pattern)
			if err != nil {
				return p, errors.New("invalid audit regex")
			}
			rules = append(rules, applied{set.source, r, re})
		}
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var value any
	if dec.Decode(&value) != nil {
		p.State = "omitted_invalid_json"
		return p, nil
	}
	var extra any
	if dec.Decode(&extra) != io.EOF {
		p.State = "omitted_invalid_json"
		return p, nil
	}
	counts := make([]int, len(rules))
	nodes := 0
	mask := func(text string) string {
		var spans [][2]int
		for i, r := range rules {
			matches := r.re.FindAllStringIndex(text, 10001)
			counts[i] += len(matches)
			if len(matches) >= 10001 {
				spans = append(spans, [2]int{0, len(text)})
				continue
			}
			for _, m := range matches {
				spans = append(spans, [2]int{m[0], m[1]})
			}
		}
		if len(spans) == 0 {
			return text
		}
		sort.Slice(spans, func(i, j int) bool { return spans[i][0] < spans[j][0] })
		var out strings.Builder
		cursor := 0
		for i := 0; i < len(spans); {
			start, end := spans[i][0], spans[i][1]
			i++
			for i < len(spans) && spans[i][0] <= end {
				end = max(end, spans[i][1])
				i++
			}
			out.WriteString(text[cursor:start])
			out.WriteString("[REDACTED]")
			cursor = end
		}
		out.WriteString(text[cursor:])
		return out.String()
	}
	var walk func(any, int) (any, error)
	walk = func(v any, depth int) (any, error) {
		nodes++
		if depth > 64 || nodes > 100000 {
			return nil, errors.New("audit traversal limit")
		}
		switch x := v.(type) {
		case string:
			masked := mask(x)
			// MCP text blocks (including our REST adapter's result copy) can
			// contain JSON. Apply field filters inside them too, sharing the
			// outer traversal budgets and preserving the string wire type.
			text := strings.TrimSpace(x)
			if len(text) > 0 && (text[0] == '{' || text[0] == '[') && json.Valid([]byte(text)) {
				// A rule may span JSON syntax. Do not lose that protection by
				// decoding and reformatting the original string first.
				if masked != x {
					return "[REDACTED]", nil
				}
				decoder := json.NewDecoder(strings.NewReader(text))
				decoder.UseNumber()
				var embedded any
				if err := decoder.Decode(&embedded); err != nil {
					return nil, errors.New("audit embedded JSON decoding failed")
				}
				embedded, err := walk(embedded, depth+1)
				if err != nil {
					return nil, err
				}
				encoded, err := json.Marshal(embedded)
				if err != nil {
					return nil, errors.New("audit embedded JSON serialization failed")
				}
				return string(encoded), nil
			}
			return masked, nil
		case json.Number:
			masked := mask(string(x))
			if masked != string(x) {
				return "[REDACTED]", nil
			}
		case []any:
			for i := range x {
				m, e := walk(x[i], depth+1)
				if e != nil {
					return nil, e
				}
				x[i] = m
			}
		case map[string]any:
			if t, _ := x["type"].(string); t == "image" || t == "audio" {
				return map[string]any{"type": t, "auditOmitted": "binary"}, nil
			}
			for k, v := range x {
				if secretKey(k) || k == "blob" {
					x[k] = "[REDACTED]"
					p.Redactions++
					continue
				}
				if mask(k) != k {
					return "[REDACTED:object]", nil
				}
				m, e := walk(v, depth+1)
				if e != nil {
					return nil, e
				}
				x[k] = m
			}
		}
		return v, nil
	}
	value, err := walk(value, 0)
	if err != nil {
		return p, err
	}
	for i, n := range counts {
		if n > 0 {
			p.Matches = append(p.Matches, RuleMatch{rules[i].source, rules[i].rule.Name, n})
			p.Redactions += n
		}
	}
	out, err := json.Marshal(value)
	if err != nil {
		return p, errors.New("audit serialization failed")
	}
	p.State = "captured"
	if p.Redactions > 0 {
		p.State = "redacted"
	}
	if len(out) > s.MaxPayloadBytes {
		p.State = "omitted_size"
		return p, nil
	}
	p.Content = out
	p.StoredBytes = len(out)
	return p, nil
}
