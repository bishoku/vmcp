package restapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"sync"
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

func TestNestedResponseProjectionPreservesShapeAndSchema(t *testing.T) {
	t.Setenv("VMCP_EGRESS_ALLOW_CIDRS", "127.0.0.0/8")
	cases := []struct {
		name, schema, body, expected string
		paths                        [][]string
	}{
		{"nested-array", `{"type":"object","properties":{"order":{"type":"object","properties":{"id":{"type":"integer"},"secret":{"type":"string"},"items":{"type":"array","minItems":1,"uniqueItems":true,"items":{"type":"object","properties":{"productId":{"type":"string"},"quantity":{"type":"integer"},"cost":{"type":"number"}},"required":["productId","cost"]}}},"required":["id","secret","items"]},"private":{"type":"string"}},"required":["order","private"]}`, `{"order":{"id":1,"secret":"hidden","items":[{"productId":"A","quantity":2,"cost":9},{"productId":"A","quantity":2,"cost":10}]},"private":"hidden"}`, `{"result":{"order":{"id":1,"items":[{"productId":"A","quantity":2},{"productId":"A","quantity":2}]}}}`, [][]string{{"order", "id"}, {"order", "items", "productId"}, {"order", "items", "quantity"}}},
		{"root-array", `{"type":"array","items":{"type":"object","properties":{"id":{"type":"integer"},"hidden":{"type":"string"},"tags":{"type":"array","items":{"type":"object","properties":{"name":{"type":"string"},"secret":{"type":"string"}}}}},"required":["id","hidden"]}}`, `[{"id":1,"hidden":"x","tags":[{"name":"one","secret":"x"}]},{"id":2,"hidden":"y","tags":[]}]`, `{"result":[{"id":1,"tags":[{"name":"one"}]},{"id":2,"tags":[]}]}`, [][]string{{"id"}, {"tags", "name"}}},
		{"nullable-missing-empty", `{"type":"object","properties":{"profile":{"type":["object","null"],"properties":{"name":{"type":"string"},"hidden":{}}},"rows":{"type":"array","items":{"type":["object","null"],"properties":{"id":{"type":"integer"},"hidden":{}}}}}}`, `{"profile":null,"rows":[null,{}, {"id":3,"hidden":"x"}],"unknown":"omitted"}`, `{"result":{"profile":null,"rows":[null,{}, {"id":3}]}}`, [][]string{{"profile", "name"}, {"rows", "id"}}},
		{"matrix", `{"type":"array","items":{"type":"array","items":{"type":"object","properties":{"id":{"type":"integer"},"hidden":{}}}}}`, `[[{"id":1,"hidden":"x"}],[],[{"id":2}]]`, `{"result":[[{"id":1}],[],[{"id":2}]]}`, [][]string{{"id"}}},
		{"literal-property-names", `{"type":"object","properties":{"a.b":{"type":"object","properties":{"/name":{"type":"string"},"*":{"type":"integer"},"hidden":{}}}}}`, `{"a.b":{"/name":"Ada","*":9007199254740993,"hidden":"x"}}`, `{"result":{"a.b":{"/name":"Ada","*":9007199254740993}}}`, [][]string{{"a.b", "/name"}, {"a.b", "*"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(tc.body)) }))
			defer server.Close()
			cfg := baseConfig()
			cfg.Operations[0].ResponseSchema = json.RawMessage(tc.schema)
			cfg.Operations[0].ResponseFieldPaths = tc.paths
			prepared := compile(t, server.URL, cfg)
			result, err := prepared.Execute(context.Background(), server.URL, "lookup", json.RawMessage(`{"id":1}`), nil, tlsconf.Settings{}, Identity{}, nil)
			if err != nil {
				t.Fatal(err)
			}
			var envelope struct {
				StructuredContent json.RawMessage `json:"structuredContent"`
				Content           []struct {
					Text string `json:"text"`
				} `json:"content"`
				IsError bool `json:"isError"`
			}
			if err = json.Unmarshal(result, &envelope); err != nil || envelope.IsError || len(envelope.Content) != 1 {
				t.Fatalf("unexpected response %s %v", result, err)
			}
			var expected, structured, text any
			_ = decodeJSON([]byte(tc.expected), &expected)
			_ = decodeJSON(envelope.StructuredContent, &structured)
			_ = decodeJSON([]byte(envelope.Content[0].Text), &text)
			if !reflect.DeepEqual(expected, structured) || !reflect.DeepEqual(expected, text) {
				t.Fatalf("projection mismatch: %s", result)
			}
			advertised, err := compileSchema(prepared.Tools[0].OutputSchema)
			if err != nil {
				t.Fatal(err)
			}
			if err = advertised.Validate(structured); err != nil {
				t.Fatalf("advertised schema mismatch %v: %s", err, prepared.Tools[0].OutputSchema)
			}
		})
	}
}

func TestProjectionRejectsInvalidAndAmbiguousPaths(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"order":{"type":"object","properties":{"id":{"type":"integer"}}},"ambiguous":{"anyOf":[{"type":"object","properties":{"id":{"type":"integer"}}},{"type":"null"}]}}}`)
	for _, paths := range [][][]string{{{}}, {{"missing"}}, {{"order", "id", "bad"}}, {{"order"}, {"order", "id"}}, {{"order", "id"}, {"order", "id"}}, {{"ambiguous", "id"}}} {
		cfg := baseConfig()
		cfg.Operations[0].ResponseSchema = schema
		cfg.Operations[0].ResponseFieldPaths = paths
		if _, err := Compile("https://example.com", cfg, nil); err == nil {
			t.Fatalf("invalid selection accepted: %#v", paths)
		}
	}
	cfg := baseConfig()
	cfg.Operations[0].ResponseFieldPaths = [][]string{{"id"}}
	cfg.Operations[0].ResponseFields = []string{"id"}
	if _, err := Compile("https://example.com", cfg, nil); err == nil {
		t.Fatal("mixed legacy/nested fields accepted")
	}
	cfg = baseConfig()
	cfg.Operations[0].ResponseSchema = schema
	cfg.Operations[0].ResponseFieldPaths = [][]string{{"ambiguous"}}
	if _, err := Compile("https://example.com", cfg, nil); err != nil {
		t.Fatal("whole complex field rejected", err)
	}
}

func TestProjectionMatchesSharedBrowserFixtures(t *testing.T) {
	data, err := os.ReadFile("../../../web/src/connections/testdata/response-projection.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []struct {
		Name           string
		Schema         any
		Paths          [][]string
		ExpectedSchema any
		Body           any
		Expected       any
	}
	if err = json.Unmarshal(data, &fixtures); err != nil {
		t.Fatal(err)
	}
	for _, fixture := range fixtures {
		t.Run(fixture.Name, func(t *testing.T) {
			sourceBefore, _ := json.Marshal(fixture.Schema)
			bodyBefore, _ := json.Marshal(fixture.Body)
			projection, shape, err := prepareProjection(fixture.Schema, nil, fixture.Paths)
			if err != nil {
				t.Fatal(err)
			}
			// Normalize Go's []string required lists to JSON's []any before comparison.
			raw, _ := json.Marshal(shape)
			var normalized any
			_ = json.Unmarshal(raw, &normalized)
			if !reflect.DeepEqual(normalized, fixture.ExpectedSchema) {
				t.Fatalf("browser schema differs: %s", raw)
			}
			var wg sync.WaitGroup
			for i := 0; i < 32; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					result, err := projection.apply(fixture.Body, 0)
					if err != nil || !reflect.DeepEqual(result, fixture.Expected) {
						t.Errorf("projection mismatch %v %v", result, err)
					}
				}()
			}
			wg.Wait()
			sourceAfter, _ := json.Marshal(fixture.Schema)
			bodyAfter, _ := json.Marshal(fixture.Body)
			if string(sourceBefore) != string(sourceAfter) || string(bodyBefore) != string(bodyAfter) {
				t.Fatal("projection mutated input or schema")
			}
		})
	}
}
