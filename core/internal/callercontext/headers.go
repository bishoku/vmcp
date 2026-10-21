package callercontext

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// HeaderMapping copies a caller-supplied request header, never verified identity.
type HeaderMapping struct {
	Source   string `json:"source"`
	Target   string `json:"target"`
	Required bool   `json:"required,omitempty"`
}

// ReservedHeader protects both sides of the mapping from credential and transport confusion.
func ReservedHeader(name string) bool {
	h := strings.ToLower(name)
	switch h {
	case "authorization", "cookie", "set-cookie", "host", "content-length", "content-type", "content-encoding", "accept", "accept-encoding", "connection", "transfer-encoding", "upgrade", "proxy-authorization", "proxy-authenticate", "proxy-connection", "te", "trailer", "keep-alive", "forwarded", "origin", "referer", "last-event-id":
		return true
	}
	return strings.HasPrefix(h, "mcp-") || strings.HasPrefix(h, "x-vmcp-") || strings.HasPrefix(h, "x-forwarded-") || strings.HasPrefix(h, "sec-")
}

func ValidateHeaderMappings(rules []HeaderMapping, static []map[string]string, caller *Config, kind string) error {
	if len(rules) > 16 {
		return errors.New("configure at most 16 request header mappings")
	}
	targets := map[string]bool{}
	for _, rule := range rules {
		if !headerRE.MatchString(rule.Source) || !headerRE.MatchString(rule.Target) {
			return errors.New("invalid source or target header name")
		}
		if ReservedHeader(rule.Source) || ReservedHeader(rule.Target) {
			return errors.New("source and target must not be transport or gateway authentication headers")
		}
		target := strings.ToLower(rule.Target)
		if targets[target] {
			return errors.New("duplicate target header")
		}
		targets[target] = true
		for _, values := range static {
			for key := range values {
				if strings.EqualFold(key, rule.Target) {
					return fmt.Errorf("target header %s conflicts with a static header", rule.Target)
				}
			}
		}
		if kind == "rest" && caller != nil && caller.Enabled {
			for _, field := range caller.Fields {
				if strings.EqualFold(field.Header, rule.Target) {
					return fmt.Errorf("target header %s conflicts with caller context", rule.Target)
				}
			}
		}
	}
	return nil
}

// PrepareHeaders selects and clones values for a single tool invocation. It never
// copies credentials to shared clients or includes caller values in errors.
func PrepareHeaders(rules []HeaderMapping, incoming http.Header) (http.Header, error) {
	out := http.Header{}
	hop := map[string]bool{}
	for _, line := range incoming.Values("Connection") {
		for _, name := range strings.Split(line, ",") {
			hop[strings.ToLower(strings.TrimSpace(name))] = true
		}
	}
	total := 0
	for _, rule := range rules {
		if hop[strings.ToLower(rule.Source)] || hop[strings.ToLower(rule.Target)] {
			return nil, errors.New("mapped header is nominated by Connection")
		}
		values := incoming.Values(rule.Source)
		nonEmpty := false
		for _, value := range values {
			if strings.TrimSpace(value) != "" {
				nonEmpty = true
			}
		}
		if !nonEmpty {
			if rule.Required {
				return nil, fmt.Errorf("required request header %s is missing", rule.Source)
			}
			continue
		}
		size := 0
		for _, value := range values {
			size += len(value)
			for _, c := range []byte(value) {
				if (c < 32 && c != '\t') || c == 127 {
					return nil, fmt.Errorf("invalid request header %s", rule.Source)
				}
			}
		}
		total += size
		if size > 8192 || total > MaxBytes {
			return nil, errors.New("request header mapping size limit exceeded")
		}
		out[http.CanonicalHeaderKey(rule.Target)] = append([]string(nil), values...)
	}
	return out, nil
}
