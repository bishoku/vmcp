package restapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/bishoku/vmcp/core/internal/tlsconf"
)

func hiddenParameter() *bool { v := false; return &v }
func parameterValuesConfig() Config {
	c := baseConfig()
	c.Operations[0].Path = "/sample/{category}"
	c.Operations[0].ResponseSchema = json.RawMessage(`{"type":"object"}`)
	c.Operations[0].Parameters = []Parameter{
		{Name: "category", In: "path", Schema: json.RawMessage(`{"type":"string"}`), ExposeToMCP: hiddenParameter(), DefaultValue: json.RawMessage(`"retail"`)},
		{Name: "type", In: "query", Schema: json.RawMessage(`{"type":"integer","enum":[1,2]}`), ExposeToMCP: hiddenParameter(), DefaultValue: json.RawMessage(`1`)},
		{Name: "content", In: "query", Required: true, Schema: json.RawMessage(`{"type":"string"}`), DefaultValue: json.RawMessage(`"all & more"`)},
		{Name: "flag", In: "query", Schema: json.RawMessage(`{"type":"boolean"}`), DefaultValue: json.RawMessage(`true`)},
		{Name: "limit", In: "query", Schema: json.RawMessage(`{"type":"integer"}`), DefaultValue: json.RawMessage(`10`)},
		{Name: "q", In: "query", Schema: json.RawMessage(`{"type":"string","default":"annotation only"}`)},
	}
	return c
}
func TestParameterDefaultsAndFixedValues(t *testing.T) {
	t.Setenv("VMCP_EGRESS_ALLOW_CIDRS", "127.0.0.0/8")
	var calls atomic.Int32
	var expected map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/sample/retail" {
			t.Errorf("path: %s", r.URL.Path)
		}
		query := r.URL.Query()
		if len(query) != len(expected) {
			t.Errorf("unexpected query: %v", query)
		}
		for key, value := range expected {
			if !query.Has(key) || query.Get(key) != value {
				t.Errorf("query %s: %v", key, query)
			}
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	compiled := compile(t, srv.URL, parameterValuesConfig())
	var input struct {
		Properties           map[string]map[string]any
		Required             []string
		AdditionalProperties bool
	}
	_ = json.Unmarshal(compiled.Tools[0].InputSchema, &input)
	if input.Properties["type"] != nil || input.Properties["category"] != nil || len(input.Required) != 0 || input.AdditionalProperties {
		t.Fatalf("wrong input schema: %s", compiled.Tools[0].InputSchema)
	}
	if input.Properties["content"]["default"] != "all & more" {
		t.Fatal("missing advertised default")
	}
	for _, tc := range []struct {
		name, args string
		query      map[string]string
		invalid    bool
	}{
		{"fallbacks", `{}`, map[string]string{"type": "1", "content": "all & more", "flag": "true", "limit": "10"}, false},
		{"explicit zero false and empty", `{"content":"","flag":false,"limit":0,"q":"a&b=2 + / örnek"}`, map[string]string{"type": "1", "content": "", "flag": "false", "limit": "0", "q": "a&b=2 + / örnek"}, false},
		{"cannot override fixed query", `{"type":2}`, nil, true},
		{"cannot override fixed path", `{"category":"other"}`, nil, true},
		{"null does not trigger fallback", `{"content":null}`, nil, true},
		{"wrong type does not trigger fallback", `{"limit":"2"}`, nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			expected = tc.query
			before := calls.Load()
			preview, err := compiled.Preview(srv.URL, "lookup", json.RawMessage(tc.args), nil)
			if (err != nil) != tc.invalid {
				t.Fatalf("preview: %v", err)
			}
			if !tc.invalid {
				req, err := compiled.Build(srv.URL, "lookup", json.RawMessage(tc.args), nil, Identity{})
				if err != nil || preview.URL != req.URL.String() {
					t.Fatal("preview/runtime mismatch", err)
				}
			}
			raw, err := compiled.Execute(context.Background(), srv.URL, "lookup", json.RawMessage(tc.args), nil, tlsconf.Settings{}, Identity{}, nil)
			if err != nil || strings.Contains(string(raw), `"isError":true`) != tc.invalid {
				t.Fatalf("execute: %s %v", raw, err)
			}
			want := before + 1
			if tc.invalid {
				want = before
			}
			if calls.Load() != want {
				t.Fatal("unexpected dispatch count")
			}
		})
	}
}
func TestDefaultParameterValidation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*Config)
	}{
		{"missing fixed value", func(c *Config) { c.Operations[0].Parameters[1].DefaultValue = nil }},
		{"enum mismatch", func(c *Config) { c.Operations[0].Parameters[1].DefaultValue = json.RawMessage(`3`) }},
		{"null default", func(c *Config) { c.Operations[0].Parameters[1].DefaultValue = json.RawMessage(`null`) }},
		{"object default", func(c *Config) { c.Operations[0].Parameters[1].DefaultValue = json.RawMessage(`{}`) }},
		{"integer mismatch", func(c *Config) { c.Operations[0].Parameters[1].DefaultValue = json.RawMessage(`1.5`) }},
		{"pattern mismatch", func(c *Config) {
			c.Operations[0].Parameters[2].Schema = json.RawMessage(`{"type":"string","pattern":"^z"}`)
		}},
		{"duplicate hidden name", func(c *Config) {
			c.Operations[0].Parameters = append(c.Operations[0].Parameters, c.Operations[0].Parameters[1])
		}},
		{"unsafe fixed path", func(c *Config) { c.Operations[0].Parameters[0].DefaultValue = json.RawMessage(`"../x"`) }},
		{"unsafe fallback path", func(c *Config) {
			c.Operations[0].Parameters[0].ExposeToMCP = nil
			c.Operations[0].Parameters[0].DefaultValue = json.RawMessage(`"%2f"`)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := parameterValuesConfig()
			tc.mutate(&c)
			if _, err := Compile("https://example.com", c, nil); err == nil {
				t.Fatal("invalid configuration accepted")
			}
		})
	}
}
func TestDefaultPathAndScalarPrecision(t *testing.T) {
	c := baseConfig()
	c.Operations[0].Parameters[0].DefaultValue = json.RawMessage(`9007199254740993`)
	c.Operations[0].Parameters[0].Required = true
	compiled := compile(t, "https://example.com", c)
	for _, tc := range []struct{ args, path string }{{`{}`, "/items/9007199254740993"}, {`{"id":0}`, "/items/0"}} {
		req, err := compiled.Build("https://example.com", "lookup", json.RawMessage(tc.args), nil, Identity{})
		if err != nil || req.URL.Path != tc.path {
			t.Fatalf("path default lost precision: %v %v", req, err)
		}
	}
	if !reflect.DeepEqual(compiled.Operations["lookup"].ParameterDefaults, map[string]any{"id": json.Number("9007199254740993")}) {
		t.Fatal("default mutated")
	}
}

func TestConfiguredFalsyDefaultsAreNotOmitted(t *testing.T) {
	c := parameterValuesConfig()
	for i := range c.Operations[0].Parameters {
		p := &c.Operations[0].Parameters[i]
		switch p.Name {
		case "content":
			p.DefaultValue = json.RawMessage(`""`)
		case "flag":
			p.DefaultValue = json.RawMessage(`false`)
		case "limit":
			p.DefaultValue = json.RawMessage(`0`)
		}
	}
	compiled := compile(t, "https://example.com", c)
	req, err := compiled.Build("https://example.com", "lookup", json.RawMessage(`{}`), nil, Identity{})
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{"content": "", "flag": "false", "limit": "0"} {
		if !req.URL.Query().Has(name) || req.URL.Query().Get(name) != want {
			t.Fatalf("missing false-like default %s: %s", name, req.URL)
		}
	}
}
