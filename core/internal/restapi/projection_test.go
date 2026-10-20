package restapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/bishoku/vmcp/core/internal/tlsconf"
)

func TestSelectedFieldsMatchAdvertisedSchemaAndBothContentForms(t *testing.T) {
	t.Setenv("VMCP_EGRESS_ALLOW_CIDRS", "127.0.0.0/8")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":7,"profile":{"name":"Ada","tags":["a","b"]},"nullable":null,"secret":"hidden","extra":"undeclared"}`))
	}))
	defer srv.Close()
	for _, tc := range []struct {
		name     string
		fields   []string
		expected string
	}{
		{"selected", []string{"id", "profile", "nullable", "optional"}, `{"result":{"id":7,"profile":{"name":"Ada","tags":["a","b"]},"nullable":null}}`},
		{"full", nil, `{"result":{"id":7,"profile":{"name":"Ada","tags":["a","b"]},"nullable":null,"secret":"hidden","extra":"undeclared"}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := baseConfig()
			cfg.Operations[0].ResponseSchema = json.RawMessage(`{"type":"object","properties":{"id":{"type":"integer"},"profile":{"type":"object","properties":{"name":{"type":"string"},"tags":{"type":"array","items":{"type":"string"}}}},"nullable":{"type":["null","string"]},"secret":{"type":"string"},"optional":{"type":"string"}},"required":["id","profile","secret"]}`)
			cfg.Operations[0].ResponseFields = tc.fields
			prepared := compile(t, srv.URL, cfg)
			raw, err := prepared.Execute(context.Background(), srv.URL, "lookup", json.RawMessage(`{"id":7}`), nil, tlsconf.Settings{}, Identity{}, nil)
			if err != nil {
				t.Fatal(err)
			}
			var result struct {
				StructuredContent any `json:"structuredContent"`
				Content           []struct {
					Text string `json:"text"`
				} `json:"content"`
				IsError bool `json:"isError"`
			}
			if err := json.Unmarshal(raw, &result); err != nil {
				t.Fatal(err)
			}
			var expected, text any
			_ = json.Unmarshal([]byte(tc.expected), &expected)
			if result.IsError || len(result.Content) != 1 {
				t.Fatalf("unexpected response: %s", raw)
			}
			if err := json.Unmarshal([]byte(result.Content[0].Text), &text); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(result.StructuredContent, expected) || !reflect.DeepEqual(text, expected) {
				t.Fatalf("projection mismatch: %s", raw)
			}
			schema, err := compileSchema(prepared.Tools[0].OutputSchema)
			if err != nil {
				t.Fatal(err)
			}
			if err := schema.Validate(result.StructuredContent); err != nil {
				t.Fatalf("output differs from advertised schema: %v", err)
			}
			if tc.name == "selected" {
				var shape struct {
					Properties map[string]struct {
						Properties           map[string]any
						Required             []string
						AdditionalProperties bool
					}
				}
				_ = json.Unmarshal(prepared.Tools[0].OutputSchema, &shape)
				projected := shape.Properties["result"]
				if len(projected.Properties) != 4 || !reflect.DeepEqual(projected.Required, []string{"id", "profile"}) || projected.AdditionalProperties {
					t.Fatalf("wrong output shape: %s", prepared.Tools[0].OutputSchema)
				}
			}
		})
	}
}
