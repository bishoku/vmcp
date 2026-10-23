// Package mockupstream manages scoped WireMock mappings. It has no MCP or store dependencies.
package mockupstream

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

type Config struct {
	Namespace string `json:"namespace,omitempty"`
}
type Rule struct {
	Name     string                     `json:"name"`
	Priority int                        `json:"priority"`
	Request  map[string]json.RawMessage `json:"request,omitempty"`
	Response map[string]json.RawMessage `json:"response"`
}
type Operation struct {
	Name, Method, Path string
	Enabled            bool
	Rules              []Rule
}
type Metadata struct {
	Owner       string `json:"owner"`
	Namespace   string `json:"namespace"`
	Fingerprint string `json:"fingerprint"`
}
type Mapping struct {
	ID       string                     `json:"id"`
	Name     string                     `json:"name"`
	Priority int                        `json:"priority"`
	Request  map[string]any             `json:"request"`
	Response map[string]json.RawMessage `json:"response"`
	Metadata map[string]Metadata        `json:"metadata"`
}

var namespacePattern = regexp.MustCompile(`^[a-f0-9]{32}$`)

func Owner(tenant string) string {
	h := sha256.Sum256([]byte(tenant))
	return hex.EncodeToString(h[:16])
}
func Namespace(tenant, slug string) string {
	h := sha256.Sum256([]byte(tenant + "\x00" + slug))
	return hex.EncodeToString(h[:16])
}
func Prefix(namespace string) string { return "/__vmcp/mock/" + namespace }
func Validate(config *Config, operations []Operation) error {
	total := 0
	for _, op := range operations {
		if config == nil {
			if len(op.Rules) > 0 {
				return errors.New("mock response rules require a mock upstream")
			}
			continue
		}
		if !namespacePattern.MatchString(config.Namespace) {
			return errors.New("invalid managed mock namespace")
		}
		if op.Enabled && len(op.Rules) == 0 {
			return fmt.Errorf("%s: enabled mock operations require at least one response rule", op.Name)
		}
		if len(op.Rules) > 20 {
			return errors.New("at most 20 mock rules per operation")
		}
		names := map[string]bool{}
		for _, rule := range op.Rules {
			if strings.TrimSpace(rule.Name) == "" || len(rule.Name) > 128 || names[rule.Name] {
				return errors.New("mock rule names must be nonempty, unique within an operation, and at most 128 bytes")
			}
			names[rule.Name] = true
			if rule.Priority < 1 || rule.Priority > 1000 {
				return errors.New("mock rule priority must be 1–1000 (1 is highest)")
			}
			for key, value := range rule.Request {
				if key != "headers" && key != "queryParameters" && key != "bodyPatterns" {
					return fmt.Errorf("unsupported mock request field %q; method and path are managed by the operation", key)
				}
				var v any
				if json.Unmarshal(value, &v) != nil || v == nil {
					return errors.New("invalid mock request matcher")
				}
				if key == "bodyPatterns" {
					if _, ok := v.([]any); !ok {
						return errors.New("bodyPatterns must be an array")
					}
				} else {
					if _, ok := v.(map[string]any); !ok {
						return errors.New("headers and queryParameters must be matcher objects")
					}
				}
			}
			if rule.Response == nil {
				return errors.New("mock response is required")
			}
			for key := range rule.Response {
				switch key {
				case "status", "headers", "jsonBody", "body", "fixedDelayMilliseconds", "transformers":
				default:
					return fmt.Errorf("unsupported mock response field %q", key)
				}
			}
			var status int
			if json.Unmarshal(rule.Response["status"], &status) != nil || status < 200 || status > 599 {
				return errors.New("mock response status must be 200–599")
			}
			if raw, ok := rule.Response["fixedDelayMilliseconds"]; ok {
				var delay int
				if json.Unmarshal(raw, &delay) != nil || delay < 0 || delay > 120000 {
					return errors.New("mock response delay must be 0–120000 milliseconds")
				}
			}
			if raw, ok := rule.Response["body"]; ok {
				var body string
				if json.Unmarshal(raw, &body) != nil {
					return errors.New("mock response body must be a string")
				}
				if _, ok := rule.Response["jsonBody"]; ok {
					return errors.New("choose body or jsonBody, not both")
				}
			}
			if raw, ok := rule.Response["transformers"]; ok {
				var transformers []string
				if json.Unmarshal(raw, &transformers) != nil || len(transformers) != 1 || transformers[0] != "response-template" {
					return errors.New("only the response-template transformer is supported")
				}
			}
			if raw, ok := rule.Response["headers"]; ok {
				var headers map[string]string
				if json.Unmarshal(raw, &headers) != nil || headers == nil {
					return errors.New("mock response headers must be a string map")
				}
				for name, value := range headers {
					if strings.ContainsAny(name+value, "\r\n\x00") {
						return errors.New("invalid mock response header")
					}
				}
			}
		}
		total += len(op.Rules)
	}
	if total > 1000 {
		return errors.New("at most 1000 mock rules per upstream")
	}
	return nil
}
func Mappings(owner string, config Config, operations []Operation) []Mapping {
	mappings := []Mapping{}
	placeholder := regexp.MustCompile(`\{[A-Za-z0-9_.-]+\}`)
	for _, op := range operations {
		if !op.Enabled {
			continue
		}
		for _, rule := range op.Rules {
			path := Prefix(config.Namespace) + op.Path
			request := map[string]any{"method": op.Method}
			if placeholder.MatchString(path) {
				parts := placeholder.Split(path, -1)
				for i := range parts {
					parts[i] = regexp.QuoteMeta(parts[i])
				}
				request["urlPathPattern"] = "^" + strings.Join(parts, "[^/]+") + "$"
			} else {
				request["urlPath"] = path
			}
			for k, v := range rule.Request {
				request[k] = v
			}
			h := sha256.Sum256([]byte(config.Namespace + "\x00" + op.Name + "\x00" + rule.Name))
			h[6] = (h[6] & 15) | 0x50
			h[8] = (h[8] & 63) | 0x80
			id := fmt.Sprintf("%x-%x-%x-%x-%x", h[0:4], h[4:6], h[6:8], h[8:10], h[10:16])
			m := Mapping{ID: id, Name: op.Name + " / " + rule.Name, Priority: rule.Priority, Request: request, Response: rule.Response}
			raw, _ := json.Marshal(m)
			fingerprint := sha256.Sum256(raw)
			m.Metadata = map[string]Metadata{"vmcp": {Owner: owner, Namespace: config.Namespace, Fingerprint: hex.EncodeToString(fingerprint[:])}}
			mappings = append(mappings, m)
		}
	}
	return mappings
}
