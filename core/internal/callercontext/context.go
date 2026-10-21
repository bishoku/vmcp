// Package callercontext carries caller-supplied data, not verified identity.
package callercontext

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

const MaxBytes = 32768

type Field struct {
	Name     string `json:"name"`
	Header   string `json:"header,omitempty"`
	Bearer   bool   `json:"bearer,omitempty"`
	Required bool   `json:"required,omitempty"`
}
type Config struct {
	Enabled bool    `json:"enabled"`
	MetaKey string  `json:"metaKey,omitempty"`
	Fields  []Field `json:"fields,omitempty"`
}

var namespaceRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*/[A-Za-z0-9][A-Za-z0-9._/-]*$`)
var fieldRE = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)
var headerRE = regexp.MustCompile("^[!#$%&'*+.^_`|~0-9A-Za-z-]{1,128}$")

// Validate checks destinations, never token contents. Static headers and managed
// authentication must not compete with dynamic headers.
func (c *Config) Validate(kind string, static []map[string]string, managedAuth bool) error {
	if c == nil || !c.Enabled {
		return nil
	}
	if len(c.MetaKey) > 256 || !namespaceRE.MatchString(c.MetaKey) {
		return errors.New("metaKey must be a namespaced key (maximum 256 bytes)")
	}
	if len(c.Fields) == 0 || len(c.Fields) > 16 {
		return errors.New("configure between 1 and 16 fields")
	}
	names, headers := map[string]bool{}, map[string]bool{}
	for _, f := range c.Fields {
		if !fieldRE.MatchString(f.Name) || names[f.Name] {
			return errors.New("invalid or duplicate field name")
		}
		names[f.Name] = true
		if kind != "rest" {
			if f.Header != "" || f.Bearer {
				return errors.New("header mappings are only supported for REST upstreams")
			}
			continue
		}
		h := strings.ToLower(f.Header)
		if !headerRE.MatchString(f.Header) || headers[h] {
			return errors.New("invalid or duplicate target header")
		}
		headers[h] = true
		switch h {
		case "host", "content-length", "connection", "transfer-encoding", "upgrade", "proxy-authorization", "proxy-connection", "te", "trailer", "keep-alive", "content-type", "accept":
			return errors.New("target header is transport managed")
		}
		if strings.HasPrefix(h, "x-vmcp-") {
			return errors.New("X-VMCP headers are reserved")
		}
		if f.Bearer && h != "authorization" {
			return errors.New("Bearer prefix requires Authorization header")
		}
		if managedAuth && h == "authorization" {
			return errors.New("Authorization conflicts with managed authentication")
		}
		for _, values := range static {
			for key := range values {
				if strings.EqualFold(key, f.Header) {
					return fmt.Errorf("target header %s conflicts with a static header", f.Header)
				}
			}
		}
	}
	return nil
}

// Prepare selects string fields from one namespace. Unknown metadata is omitted.
// Errors never contain caller-provided values.
func (c *Config) Prepare(raw json.RawMessage, kind string) (json.RawMessage, http.Header, error) {
	if c == nil || !c.Enabled {
		return nil, nil, nil
	}
	if len(raw) > MaxBytes {
		return nil, nil, errors.New("caller metadata exceeds 32 KiB")
	}
	var meta map[string]json.RawMessage
	if len(raw) > 0 && json.Unmarshal(raw, &meta) != nil {
		return nil, nil, errors.New("caller metadata must be an object")
	}
	var values map[string]json.RawMessage
	if value, ok := meta[c.MetaKey]; ok {
		if string(value) == "null" || json.Unmarshal(value, &values) != nil || values == nil {
			return nil, nil, errors.New("caller context must be an object")
		}
	}
	selected := map[string]string{}
	headers := http.Header{}
	for _, f := range c.Fields {
		value, ok := values[f.Name]
		if !ok {
			if f.Required {
				return nil, nil, fmt.Errorf("required caller context field %s is missing", f.Name)
			}
			continue
		}
		var s string
		if string(value) == "null" || json.Unmarshal(value, &s) != nil || len(s) > 8192 || strings.ContainsAny(s, "\r\n\x00") {
			return nil, nil, fmt.Errorf("invalid caller context field %s", f.Name)
		}
		if s == "" {
			if f.Required {
				return nil, nil, fmt.Errorf("required caller context field %s is empty", f.Name)
			}
			continue
		}
		selected[f.Name] = s
		if kind == "rest" {
			if f.Bearer {
				s = "Bearer " + s
			}
			if len(s) > 8192 {
				return nil, nil, errors.New("caller context header exceeds 8 KiB")
			}
			headers.Set(f.Header, s)
		}
	}
	if len(selected) == 0 {
		return nil, headers, nil
	}
	out, _ := json.Marshal(map[string]any{c.MetaKey: selected})
	return out, headers, nil
}

type headerKey struct{}

func WithHeaders(ctx context.Context, headers http.Header) context.Context {
	return context.WithValue(ctx, headerKey{}, headers.Clone())
}
func ApplyHeaders(ctx context.Context, req *http.Request) {
	if h, ok := ctx.Value(headerKey{}).(http.Header); ok {
		for k, v := range h {
			req.Header[k] = append([]string(nil), v...)
		}
	}
}
