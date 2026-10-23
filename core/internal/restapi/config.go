// Package restapi implements operator-defined, bounded HTTP operations as MCP tools.
package restapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/bishoku/vmcp/core/internal/mcp"
	"github.com/bishoku/vmcp/core/internal/mockupstream"
	"github.com/bishoku/vmcp/core/internal/netpolicy"
	"github.com/bishoku/vmcp/core/internal/oauthtoken"
	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
)

type Auth struct {
	Type            string   `json:"type"`
	Username        string   `json:"username,omitempty"`
	Password        string   `json:"password,omitempty"`
	Token           string   `json:"token,omitempty"`
	TokenURL        string   `json:"tokenUrl,omitempty"`
	ClientID        string   `json:"clientId,omitempty"`
	ClientSecret    string   `json:"clientSecret,omitempty"`
	Scopes          []string `json:"scopes,omitempty"`
	Audience        string   `json:"audience,omitempty"`
	TokenAuthMethod string   `json:"tokenAuthMethod,omitempty"`
	HasPassword     bool     `json:"hasPassword,omitempty"`
	HasToken        bool     `json:"hasToken,omitempty"`
	HasClientSecret bool     `json:"hasClientSecret,omitempty"`
}
type Parameter struct {
	Name         string          `json:"name"`
	In           string          `json:"in"`
	Required     bool            `json:"required"`
	Schema       json.RawMessage `json:"schema"`
	ExposeToMCP  *bool           `json:"exposeToMcp,omitempty"`
	DefaultValue json.RawMessage `json:"defaultValue,omitempty"`
}
type Operation struct {
	MockRules          []mockupstream.Rule `json:"mockRules,omitempty"`
	Name               string              `json:"name"`
	Description        string              `json:"description"`
	Enabled            bool                `json:"enabled"`
	Method             string              `json:"method"`
	Path               string              `json:"path"`
	Parameters         []Parameter         `json:"parameters"`
	BodySchema         json.RawMessage     `json:"bodySchema,omitempty"`
	ResponseSchema     json.RawMessage     `json:"responseSchema,omitempty"`
	ResponseFieldPaths [][]string          `json:"responseFieldPaths,omitempty"`
	ResponseFields     []string            `json:"responseFields,omitempty"`
	SuccessStatuses    []int               `json:"successStatuses,omitempty"`
	Headers            map[string]string   `json:"headers,omitempty"`
	HeaderNames        []string            `json:"headerNames,omitempty"`
	ReadOnly           bool                `json:"readOnly"`
}
type Config struct {
	Mock              *mockupstream.Config `json:"mock,omitempty"`
	Auth              Auth                 `json:"auth"`
	ForwardIdentity   bool                 `json:"forwardIdentity"`
	RequireIdentity   bool                 `json:"requireIdentity"`
	AllowInsecureHTTP bool                 `json:"allowInsecureHttp"`
	TimeoutSeconds    int                  `json:"timeoutSeconds"`
	Operations        []Operation          `json:"operations"`
}
type Compiled struct {
	Config     Config                       `json:"config"`
	Operations map[string]PreparedOperation `json:"-"`
	Tools      []mcp.Tool                   `json:"-"`
}
type PreparedOperation struct {
	Definition        Operation
	Input             *jsonschema.Schema
	Output            *jsonschema.Schema
	ParameterDefaults map[string]any
	Projection        *projectionNode
}

func (a Auth) OAuth() *oauthtoken.Config {
	if a.Type != "oauth2" {
		return nil
	}
	return &oauthtoken.Config{TokenURL: a.TokenURL, ClientID: a.ClientID, ClientSecret: a.ClientSecret, Scopes: a.Scopes, Audience: a.Audience, AuthStyle: a.TokenAuthMethod}
}

// Resolve preserves omitted secrets and operation headers by stable operation name.
func Resolve(in *Config, previous *Config) *Config {
	if in == nil {
		return previous
	}
	raw, _ := json.Marshal(in)
	var c Config
	_ = json.Unmarshal(raw, &c)
	if c.Auth.Type == "" {
		c.Auth.Type = "none"
	}
	c.Auth.HasPassword = false
	c.Auth.HasToken = false
	c.Auth.HasClientSecret = false
	if c.TimeoutSeconds == 0 {
		c.TimeoutSeconds = 30
	}
	if c.Auth.Type == "oauth2" && c.Auth.TokenAuthMethod == "" {
		c.Auth.TokenAuthMethod = "client_secret_basic"
	}
	if previous != nil {
		if c.Auth.Type == previous.Auth.Type {
			if c.Auth.Password == "" {
				c.Auth.Password = previous.Auth.Password
			}
			if c.Auth.Token == "" {
				c.Auth.Token = previous.Auth.Token
			}
			if c.Auth.ClientSecret == "" {
				c.Auth.ClientSecret = previous.Auth.ClientSecret
			}
		}
		for i := range c.Operations {
			if in.Operations[i].Headers != nil {
				c.Operations[i].Headers = maps.Clone(in.Operations[i].Headers)
			} else if c.Operations[i].Headers == nil {
				for _, old := range previous.Operations {
					if old.Name == c.Operations[i].Name {
						c.Operations[i].Headers = maps.Clone(old.Headers)
					}
				}
			}
		}
	}
	switch c.Auth.Type {
	case "none":
		c.Auth = Auth{Type: "none"}
	case "basic":
		c.Auth = Auth{Type: "basic", Username: c.Auth.Username, Password: c.Auth.Password}
	case "bearer":
		c.Auth = Auth{Type: "bearer", Token: c.Auth.Token}
	case "oauth2":
		c.Auth.Scopes = strings.Fields(strings.Join(c.Auth.Scopes, " "))
		c.Auth.Password = ""
		c.Auth.Token = ""
		c.Auth.Username = ""
	}
	if c.Operations == nil {
		c.Operations = []Operation{}
	}
	for i := range c.Operations {
		c.Operations[i].HeaderNames = nil
		if c.Operations[i].Parameters == nil {
			c.Operations[i].Parameters = []Parameter{}
		}
	}
	return &c
}
func (c Config) Public() *Config {
	raw, _ := json.Marshal(c)
	var out Config
	_ = json.Unmarshal(raw, &out)
	out.Auth.HasPassword = out.Auth.Password != ""
	out.Auth.Password = ""
	out.Auth.HasToken = out.Auth.Token != ""
	out.Auth.Token = ""
	out.Auth.HasClientSecret = out.Auth.ClientSecret != ""
	out.Auth.ClientSecret = ""
	for i := range out.Operations {
		out.Operations[i].HeaderNames = slices.Sorted(maps.Keys(out.Operations[i].Headers))
		out.Operations[i].Headers = nil
	}
	return &out
}

var nameRE = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,128}$`)
var placeholderRE = regexp.MustCompile(`\{([A-Za-z0-9_.-]+)\}`)
var headerRE = regexp.MustCompile("^[!#$%&'*+.^_`|~0-9A-Za-z-]+$")

func ValidateHeaders(headers map[string]string, auth bool) error {
	seen := map[string]bool{}
	for name, value := range headers {
		k := strings.ToLower(name)
		if !headerRE.MatchString(name) || strings.ContainsAny(value, "\r\n\x00") || seen[k] || len(value) > 8192 {
			return errors.New("invalid, duplicate or oversized header")
		}
		seen[k] = true
		if strings.HasPrefix(k, "x-vmcp-") || slices.Contains([]string{"host", "content-length", "connection", "transfer-encoding", "upgrade", "proxy-authorization", "proxy-connection", "te", "trailer", "keep-alive", "content-type", "accept"}, k) {
			return fmt.Errorf("header %s is managed by vMCP", name)
		}
		if auth && k == "authorization" {
			return errors.New("Authorization conflicts with the selected authentication method")
		}
	}
	return nil
}
func compileSchema(raw json.RawMessage) (*jsonschema.Schema, error) {
	var value any
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	if len(raw) > 65536 || decodeJSON(raw, &value) != nil {
		return nil, errors.New("invalid or oversized JSON Schema")
	}
	if hasSchemaReference(value) {
		return nil, errors.New("schema references are not supported; use inline schemas")
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	compiler.UseLoader(nil)
	// No URL loader is registered: external schema references cannot access the network or filesystem.
	if err := compiler.AddResource("urn:vmcp:rest:schema", value); err != nil {
		return nil, err
	}
	return compiler.Compile("urn:vmcp:rest:schema")
}
func Compile(base string, c Config, headers map[string]string) (*Compiled, error) {
	if err := netpolicy.ValidateURL(base); err != nil {
		return nil, err
	}
	u, _ := url.Parse(base)
	if u.RawQuery != "" || u.RawPath != "" {
		return nil, errors.New("base URL cannot contain query or encoded path")
	}
	if u.Scheme != "https" && !c.AllowInsecureHTTP {
		return nil, errors.New("REST requires HTTPS; enable the explicit insecure HTTP development exception if needed")
	}
	if c.TimeoutSeconds < 1 || c.TimeoutSeconds > 120 || len(c.Operations) > 200 {
		return nil, errors.New("timeout must be 1–120 seconds; at most 200 operations")
	}
	if c.Mock != nil {
		if err := mockupstream.ValidateEndpoint(base); err != nil {
			return nil, err
		}
	}
	if err := mockupstream.Validate(c.Mock, c.MockOperations()); err != nil {
		return nil, err
	}
	raw, _ := json.Marshal(c)
	if len(raw) > 512<<10 {
		return nil, errors.New("REST configuration exceeds 512 KiB")
	}
	if c.RequireIdentity && !c.ForwardIdentity {
		return nil, errors.New("required identity needs identity forwarding enabled")
	}
	switch c.Auth.Type {
	case "none":
	case "basic":
		if c.Auth.Username == "" || c.Auth.Password == "" || strings.Contains(c.Auth.Username, ":") {
			return nil, errors.New("Basic Auth requires a username without colon and a password")
		}
	case "bearer":
		if c.Auth.Token == "" || strings.ContainsAny(c.Auth.Token, "\r\n\x00") {
			return nil, errors.New("invalid bearer token")
		}
	case "oauth2":
		oc := c.Auth.OAuth()
		if len(oc.Validate()) > 0 {
			return nil, errors.New("invalid OAuth client credentials configuration")
		}
		if !slices.Contains([]string{"client_secret_basic", "client_secret_post"}, c.Auth.TokenAuthMethod) {
			return nil, errors.New("invalid token authentication method")
		}
		tu, _ := url.Parse(c.Auth.TokenURL)
		if tu.Scheme != "https" && !c.AllowInsecureHTTP {
			return nil, errors.New("token endpoint requires HTTPS")
		}
	default:
		return nil, errors.New("unsupported REST authentication method")
	}
	if err := ValidateHeaders(headers, c.Auth.Type != "none"); err != nil {
		return nil, err
	}
	result := &Compiled{Config: c, Operations: map[string]PreparedOperation{}, Tools: []mcp.Tool{}}
	seen := map[string]bool{}
	for _, op := range c.Operations {
		if !nameRE.MatchString(op.Name) || seen[op.Name] || utf8.RuneCountInString(op.Description) > 20000 {
			return nil, errors.New("operation names must be unique and descriptions at most 20000 characters")
		}
		seen[op.Name] = true
		if !slices.Contains([]string{"GET", "POST", "PUT", "PATCH", "DELETE"}, op.Method) {
			return nil, fmt.Errorf("%s: unsupported HTTP method", op.Name)
		}
		if op.ReadOnly && op.Method != "GET" {
			return nil, errors.New("only GET operations may be marked read-only")
		}
		if !strings.HasPrefix(op.Path, "/") || strings.HasPrefix(op.Path, "//") || strings.ContainsAny(op.Path, "?#%\\\r\n") || len(op.Path) > 2048 {
			return nil, errors.New("operation path must be an absolute path without query, fragments or escapes")
		}
		for _, segment := range strings.Split(op.Path, "/") {
			if segment == "." || segment == ".." {
				return nil, errors.New("path traversal is not allowed")
			}
		}
		if err := ValidateHeaders(op.Headers, c.Auth.Type != "none"); err != nil {
			return nil, fmt.Errorf("%s: %w", op.Name, err)
		}
		properties := map[string]any{}
		required := []string{}
		paths := map[string]bool{}
		parameterNames := map[string]bool{}
		defaults := map[string]any{}
		if len(op.Parameters) > 100 {
			return nil, errors.New("at most 100 parameters")
		}
		for _, p := range op.Parameters {
			if !nameRE.MatchString(p.Name) || p.Name == "body" || parameterNames[p.Name] || !slices.Contains([]string{"path", "query"}, p.In) {
				return nil, errors.New("invalid or duplicate parameter")
			}
			parameterNames[p.Name] = true
			var schema map[string]any
			if decodeJSON(p.Schema, &schema) != nil {
				return nil, errors.New("invalid parameter schema")
			}
			typ, _ := schema["type"].(string)
			if !slices.Contains([]string{"string", "number", "integer", "boolean"}, typ) {
				return nil, errors.New("path/query parameters must have scalar types")
			}

			hasDefault := len(p.DefaultValue) > 0
			exposed := p.ExposeToMCP == nil || *p.ExposeToMCP
			if !exposed && !hasDefault {
				return nil, fmt.Errorf("%s parameter %s: hidden parameters require a fixed value", op.Name, p.Name)
			}
			if hasDefault {
				var value any
				if decodeJSON(p.DefaultValue, &value) != nil {
					return nil, fmt.Errorf("%s parameter %s: invalid default value", op.Name, p.Name)
				}
				validator, err := compileSchema(p.Schema)
				if err != nil {
					return nil, fmt.Errorf("%s parameter %s: invalid schema", op.Name, p.Name)
				}
				if err := validator.Validate(value); err != nil {
					return nil, fmt.Errorf("%s parameter %s: default value does not match parameter schema", op.Name, p.Name)
				}
				if p.In == "path" && unsafePathValue(fmt.Sprint(value)) {
					return nil, fmt.Errorf("%s parameter %s: unsafe default path value", op.Name, p.Name)
				}
				defaults[p.Name] = value
				// This annotation describes the runtime fallback; schema.default alone
				// never enables substitution for existing configurations.
				schema["default"] = value
			}
			if exposed {
				properties[p.Name] = schema
				if !hasDefault && (p.Required || p.In == "path") {
					required = append(required, p.Name)
				}
			}
			if p.In == "path" {
				paths[p.Name] = true
			}
		}
		for _, m := range placeholderRE.FindAllStringSubmatch(op.Path, -1) {
			if !paths[m[1]] {
				return nil, errors.New("path placeholder has no parameter")
			}
			delete(paths, m[1])
		}
		if len(paths) > 0 || strings.ContainsAny(placeholderRE.ReplaceAllString(op.Path, ""), "{}") {
			return nil, errors.New("unmatched path parameters")
		}
		if len(op.BodySchema) > 0 {
			if op.Method == "GET" {
				return nil, errors.New("GET request bodies are not supported")
			}
			var body any
			if decodeJSON(op.BodySchema, &body) != nil {
				return nil, errors.New("invalid body schema")
			}
			properties["body"] = body
			required = append(required, "body")
		}
		inputRaw, _ := json.Marshal(map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false})
		input, err := compileSchema(inputRaw)
		if err != nil {
			return nil, fmt.Errorf("%s input schema: %w", op.Name, err)
		}
		output, err := compileSchema(op.ResponseSchema)
		if err != nil {
			return nil, fmt.Errorf("%s response schema: %w", op.Name, err)
		}
		var outputShape any = map[string]any{}
		if len(op.ResponseSchema) > 0 {
			_ = decodeJSON(op.ResponseSchema, &outputShape)
		}
		projection, projected, err := prepareProjection(outputShape, op.ResponseFields, op.ResponseFieldPaths)
		if err != nil {
			return nil, fmt.Errorf("%s response projection: %w", op.Name, err)
		}
		outputShape = projected

		for _, status := range op.SuccessStatuses {
			if status < 200 || status > 299 {
				return nil, errors.New("success statuses must be 2xx")
			}
		}
		outputRaw, _ := json.Marshal(map[string]any{"type": "object", "properties": map[string]any{"result": outputShape}, "required": []string{"result"}, "additionalProperties": false})
		if _, err := compileSchema(outputRaw); err != nil {
			return nil, fmt.Errorf("%s projected output schema: %w", op.Name, err)
		}
		if !op.Enabled {
			continue
		}
		annotations, _ := json.Marshal(map[string]bool{"readOnlyHint": op.ReadOnly, "destructiveHint": !op.ReadOnly, "idempotentHint": false, "openWorldHint": true})
		result.Tools = append(result.Tools, mcp.Tool{Name: op.Name, Description: op.Description, InputSchema: inputRaw, OutputSchema: outputRaw, Annotations: annotations})
		result.Operations[op.Name] = PreparedOperation{Definition: op, Input: input, Output: output, ParameterDefaults: defaults, Projection: projection}
	}
	return result, nil
}

// Headers merges operator-owned values case-insensitively, with operation precedence.
func Headers(common, operation map[string]string) http.Header {
	h := make(http.Header)
	for k, v := range common {
		h.Set(k, v)
	}
	for k, v := range operation {
		h.Set(k, v)
	}
	return h
}

func hasSchemaReference(value any) bool {
	switch v := value.(type) {
	case map[string]any:
		for k, child := range v {
			if k == "$ref" || k == "$dynamicRef" || k == "$recursiveRef" {
				return true
			}
			if hasSchemaReference(child) {
				return true
			}
		}
	case []any:
		for _, child := range v {
			if hasSchemaReference(child) {
				return true
			}
		}
	}
	return false
}

func (c Config) MockOperations() []mockupstream.Operation {
	out := make([]mockupstream.Operation, 0, len(c.Operations))
	for _, op := range c.Operations {
		out = append(out, mockupstream.Operation{Name: op.Name, Method: op.Method, Path: op.Path, Enabled: op.Enabled, Rules: op.MockRules})
	}
	return out
}
