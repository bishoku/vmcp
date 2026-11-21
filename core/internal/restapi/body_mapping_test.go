package restapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/bishoku/vmcp/core/internal/tlsconf"
)

func TestBodyMappingPreservesJSONTypesAndEnforcesNestedRequired(t *testing.T) {
	t.Setenv("VMCP_EGRESS_ALLOW_CIDRS", "127.0.0.0/8")
	for _, tc := range []struct {
		name, schema, args, body string
		valid                    bool
	}{
		{"nested numeric precision", `{"type":"object"}`, `{"items":[{"productId":9007199254740993,"quantity":0,"price":12.50}],"active":false,"note":null,"tags":[]}`, `{"items":[{"productId":9007199254740993,"quantity":0,"price":12.50}],"active":false,"note":null,"tags":[]}`, true},
		{"root array", `{"type":"array","items":{"type":"object"}}`, `{"body":[{"productId":"SKU-1","quantity":2}]}`, `[{"productId":"SKU-1","quantity":2}]`, true},
		{"body named property remains inside body", `{"type":"object"}`, `{"body":{"items":[]}}`, `{"body":{"items":[]}}`, true},
		{"defaults are not inserted", `{"type":"object","properties":{"quantity":{"type":"integer","default":1}}}`, `{}`, `{}`, true},
		{"nested required is enforced", `{"type":"object","properties":{"items":{"type":"array","items":{"type":"object","properties":{"productId":{"type":"string"},"quantity":{"type":"integer"}},"required":["productId","quantity"]}}},"required":["items"]}`, `{"items":[{"productId":"SKU-1"}]}`, ``, false},
		{"unknown fields in closed bodies rejected", `{"type":"object","properties":{"quantity":{"type":"integer"}},"additionalProperties":false}`, `{"quantity":2,"productid":"SKU-1"}`, ``, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				raw, err := io.ReadAll(r.Body)
				if err != nil {
					t.Error(err)
				}
				var got, want any
				if decodeJSON(raw, &got) != nil || decodeJSON([]byte(tc.body), &want) != nil || !reflect.DeepEqual(got, want) {
					t.Errorf("body mapping: got %s want %s", raw, tc.body)
				}
				w.WriteHeader(http.StatusNoContent)
			}))
			defer srv.Close()
			c := baseConfig()
			c.Operations = []Operation{{Name: "create", Enabled: true, Method: "POST", Path: "/orders", BodySchema: json.RawMessage(tc.schema), ResponseSchema: json.RawMessage(`{"type":"null"}`)}}
			compiled := compile(t, srv.URL, c)
			out, err := compiled.Execute(context.Background(), srv.URL, "create", json.RawMessage(tc.args), nil, tlsconf.Settings{}, Identity{}, nil)
			if err != nil {
				t.Fatal(err)
			}
			if tc.valid {
				if calls.Load() != 1 || strings.Contains(string(out), `"isError":true`) {
					t.Fatalf("request failed: %s calls=%d", out, calls.Load())
				}
			} else if calls.Load() != 0 || !strings.Contains(string(out), "arguments do not match") {
				t.Fatalf("invalid request dispatched: %s calls=%d", out, calls.Load())
			}
		})
	}
}

func TestArrayArgumentAndConflictingNames(t *testing.T) {
	c := baseConfig()
	op := Operation{Name: "batch", Method: "POST", Path: "/batch", Enabled: true, BodySchema: json.RawMessage(`{"type":"array","items":{"type":"integer"}}`), BodyArgumentName: "items"}
	c.Operations = []Operation{op}
	compiled := compile(t, "https://example.com", c)
	preview, err := compiled.Preview("https://example.com", "batch", json.RawMessage(`{"items":[0,2]}`), nil)
	if err != nil || string(preview.Body) != "[0,2]" {
		t.Fatalf("named array: %s %v", preview.Body, err)
	}
	for _, args := range []string{`{"body":[1]}`, `{"items":[false]}`, `{}`} {
		if _, err := compiled.Build("https://example.com", "batch", json.RawMessage(args), nil, Identity{}); err == nil {
			t.Fatal(args)
		}
	}
	fixed := false
	op.Parameters = []Parameter{{Name: "items", In: "query", Schema: json.RawMessage(`{"type":"integer"}`), ExposeToMCP: &fixed, DefaultValue: json.RawMessage("0")}}
	for _, schema := range []string{string(op.BodySchema), `{"type":"object","properties":{"items":{}}}`} {
		op.BodySchema = json.RawMessage(schema)
		if strings.Contains(schema, "properties") {
			op.BodyArgumentName = ""
		}
		c.Operations[0] = op
		if _, err := Compile("https://example.com", c, nil); err == nil || !strings.Contains(err.Error(), `batch: argument "items" conflicts between query and request body`) {
			t.Fatal(err)
		}
	}
	op.BodySchema = json.RawMessage(`{"type":"object","additionalProperties":{"type":"integer"}}`)
	c.Operations[0] = op
	compiled = compile(t, "https://example.com", c)
	if _, err := compiled.Build("https://example.com", "batch", json.RawMessage(`{"items":42}`), nil, Identity{}); err == nil {
		t.Fatal("overrode fixed query")
	}
	preview, err = compiled.Preview("https://example.com", "batch", json.RawMessage(`{"total":3}`), nil)
	if err != nil || preview.URL != "https://example.com/batch?items=0" || string(preview.Body) != `{"total":3}` {
		t.Fatal(preview, err)
	}
}
