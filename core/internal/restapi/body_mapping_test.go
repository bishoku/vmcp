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
		{"nested numeric precision", `{"type":"object"}`, `{"body":{"items":[{"productId":9007199254740993,"quantity":0,"price":12.50}],"active":false,"note":null,"tags":[]}}`, `{"items":[{"productId":9007199254740993,"quantity":0,"price":12.50}],"active":false,"note":null,"tags":[]}`, true},
		{"root array", `{"type":"array","items":{"type":"object"}}`, `{"body":[{"productId":"SKU-1","quantity":2}]}`, `[{"productId":"SKU-1","quantity":2}]`, true},
		{"body named property remains inside body", `{"type":"object"}`, `{"body":{"body":{"items":[]}}}`, `{"body":{"items":[]}}`, true},
		{"defaults are not inserted", `{"type":"object","properties":{"quantity":{"type":"integer","default":1}}}`, `{"body":{}}`, `{}`, true},
		{"nested required is enforced", `{"type":"object","properties":{"items":{"type":"array","items":{"type":"object","properties":{"productId":{"type":"string"},"quantity":{"type":"integer"}},"required":["productId","quantity"]}}},"required":["items"]}`, `{"body":{"items":[{"productId":"SKU-1"}]}}`, ``, false},
		{"unknown fields in closed bodies rejected", `{"type":"object","properties":{"quantity":{"type":"integer"}},"additionalProperties":false}`, `{"body":{"quantity":2,"productid":"SKU-1"}}`, ``, false},
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
