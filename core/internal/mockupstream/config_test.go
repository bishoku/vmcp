package mockupstream_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/bishoku/vmcp/core/internal/mockupstream"
	"github.com/bishoku/vmcp/core/internal/restapi"
	"github.com/bishoku/vmcp/core/internal/tlsconf"
)

func raw(s string) json.RawMessage { return json.RawMessage(s) }
func rule() mockupstream.Rule {
	return mockupstream.Rule{Name: "ok", Priority: 1, Response: map[string]json.RawMessage{"status": raw(`200`), "jsonBody": raw(`{"source":"one"}`)}}
}
func TestScopedMappingsAndValidation(t *testing.T) {
	ns := mockupstream.Namespace("tenant", "one")
	cfg := &mockupstream.Config{Namespace: ns}
	ops := []mockupstream.Operation{{Name: "lookup", Method: "GET", Path: "/orders/{id}", Enabled: true, Rules: []mockupstream.Rule{rule()}}}
	if err := mockupstream.Validate(cfg, ops); err != nil {
		t.Fatal(err)
	}
	a := mockupstream.Mappings("owner", *cfg, ops)
	b := mockupstream.Mappings("owner", *cfg, ops)
	if !reflect.DeepEqual(a, b) || len(a) != 1 || a[0].Request["urlPathPattern"] != "^"+mockupstream.Prefix(ns)+"/orders/[^/]+$" {
		t.Fatalf("unstable or unsafe mapping: %+v", a)
	}
	other := mockupstream.Mappings("owner", mockupstream.Config{Namespace: mockupstream.Namespace("tenant", "two")}, ops)
	if a[0].ID == other[0].ID || a[0].Request["urlPathPattern"] == other[0].Request["urlPathPattern"] {
		t.Fatal("namespace collision")
	}
	for _, mutate := range []func(*mockupstream.Rule){
		func(r *mockupstream.Rule) { r.Request = map[string]json.RawMessage{"urlPattern": raw(`".*"`)} },
		func(r *mockupstream.Rule) { r.Response["proxyBaseUrl"] = raw(`"https://example.com"`) },
		func(r *mockupstream.Rule) { r.Response["bodyFileName"] = raw(`"secrets"`) },
		func(r *mockupstream.Rule) { r.Response["transformers"] = raw(`["custom"]`) },
		func(r *mockupstream.Rule) { r.Response["fixedDelayMilliseconds"] = raw(`120001`) },
		func(r *mockupstream.Rule) { r.Response["status"] = raw(`0`) },
		func(r *mockupstream.Rule) { r.Request = map[string]json.RawMessage{"bodyPatterns": raw(`{}`)} },
	} {
		r := rule()
		mutate(&r)
		if mockupstream.Validate(cfg, []mockupstream.Operation{{Rules: []mockupstream.Rule{r}}}) == nil {
			t.Fatalf("invalid rule accepted: %+v", r)
		}
	}
	if mockupstream.Validate(nil, ops) == nil {
		t.Fatal("normal REST upstream accepted mock rules")
	}
}
func TestWireMockClientRejectsFailureAndOversizedResponses(t *testing.T) {
	t.Setenv("VMCP_EGRESS_ALLOW_CIDRS", "127.0.0.0/8")
	for _, tc := range []struct {
		status int
		body   string
	}{{500, `{}`}, {200, `not json`}, {200, strings.Repeat(" ", 4<<20) + `{}`}} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(tc.status)
			_, _ = w.Write([]byte(tc.body))
		}))
		_, err := mockupstream.NewClient(srv.URL).List(context.Background())
		srv.Close()
		if err == nil {
			t.Fatal("invalid admin response accepted")
		}
	}
}
func TestRealWireMockScopedDynamicRESTExecution(t *testing.T) {
	base := os.Getenv("VMCP_TEST_WIREMOCK_URL")
	if base == "" {
		if os.Getenv("VMCP_REQUIRE_INTEGRATION") == "1" {
			t.Fatal("VMCP_TEST_WIREMOCK_URL required")
		}
		t.Skip("VMCP_TEST_WIREMOCK_URL not set")
	}
	t.Setenv("VMCP_EGRESS_ALLOW_CIDRS", "127.0.0.0/8")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	owner := mockupstream.Owner(fmt.Sprint("wiremock-integration-", time.Now().UnixNano()))
	client := mockupstream.NewClient(base)
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := client.Reconcile(cleanup, owner, nil); err != nil {
			t.Error(err)
		}
	})
	cfg := restapi.Config{Auth: restapi.Auth{Type: "none"}, TimeoutSeconds: 3, AllowInsecureHTTP: true, Mock: &mockupstream.Config{Namespace: mockupstream.Namespace(owner, "one")}, Operations: []restapi.Operation{
		{Name: "sample", Method: "GET", Path: "/sample", Enabled: true, ReadOnly: true, MockRules: []mockupstream.Rule{rule()}},
		{Name: "create", Method: "POST", Path: "/orders", Enabled: true, BodySchema: raw(`{"type":"object"}`), ResponseSchema: raw(`{"type":"object","properties":{"sku":{"type":"string"},"hidden":{"type":"string"}}}`), ResponseFields: []string{"sku"}, MockRules: []mockupstream.Rule{{Name: "nested", Priority: 1, Request: map[string]json.RawMessage{"bodyPatterns": raw(`[{"matchesJsonPath":"$.order.items[0].productId"}]`)}, Response: map[string]json.RawMessage{"status": raw(`200`), "jsonBody": raw(`{"sku":"{{jsonPath request.body '$.order.items[0].productId'}}","hidden":"removed"}`), "transformers": raw(`["response-template"]`)}}}},
		{Name: "slow", Method: "GET", Path: "/slow", Enabled: true, MockRules: []mockupstream.Rule{{Name: "slow", Priority: 1, Response: map[string]json.RawMessage{"status": raw(`200`), "jsonBody": raw(`{}`), "fixedDelayMilliseconds": raw(`1000`)}}}},
		{Name: "fail", Method: "GET", Path: "/fail", Enabled: true, MockRules: []mockupstream.Rule{{Name: "fail", Priority: 1, Response: map[string]json.RawMessage{"status": raw(`503`), "jsonBody": raw(`{"error":"simulated"}`)}}}},
	}}
	compiled, err := restapi.Compile(base, cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	desired := mockupstream.Mappings(owner, *cfg.Mock, cfg.MockOperations())
	otherCfg := restapi.Config{Auth: restapi.Auth{Type: "none"}, TimeoutSeconds: 3, AllowInsecureHTTP: true, Mock: &mockupstream.Config{Namespace: mockupstream.Namespace(owner, "two")}, Operations: []restapi.Operation{{Name: "sample", Method: "GET", Path: "/sample", Enabled: true, MockRules: []mockupstream.Rule{{Name: "two", Priority: 1, Response: map[string]json.RawMessage{"status": raw(`200`), "jsonBody": raw(`{"source":"two"}`)}}}}}}
	other, err := restapi.Compile(base, otherCfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	otherMappings := mockupstream.Mappings(owner, *otherCfg.Mock, otherCfg.MockOperations())
	desired = append(desired, otherMappings...)
	if err := client.Reconcile(ctx, owner, desired); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		compiled         *restapi.Compiled
		name, args, want string
	}{{compiled, "sample", `{}`, `"source":"one"`}, {other, "sample", `{}`, `"source":"two"`}, {compiled, "create", `{"body":{"order":{"items":[{"productId":"SKU-42"}]}}}`, `"sku":"SKU-42"`}, {compiled, "fail", `{}`, `HTTP 503`}} {
		result, err := tc.compiled.Execute(ctx, base, tc.name, raw(tc.args), nil, tlsconf.Settings{}, restapi.Identity{}, nil)
		if err != nil || !strings.Contains(string(result), tc.want) || strings.Contains(string(result), `"hidden"`) {
			t.Fatalf("%s: %s %v", tc.name, result, err)
		}
	}
	short, stop := context.WithTimeout(ctx, 50*time.Millisecond)
	defer stop()
	if _, err := compiled.Execute(short, base, "slow", raw(`{}`), nil, tlsconf.Settings{}, restapi.Identity{}, nil); err == nil {
		t.Fatal("delay ignored cancellation")
	}
	// Delete one namespace while preserving the other, then recreate the inventory
	// to exercise the same recovery used after a WireMock restart.
	if err := client.Reconcile(ctx, owner, otherMappings); err != nil {
		t.Fatal(err)
	}
	result, err := other.Execute(ctx, base, "sample", raw(`{}`), nil, tlsconf.Settings{}, restapi.Identity{}, nil)
	if err != nil || !strings.Contains(string(result), `"source":"two"`) {
		t.Fatal("deletion affected another mock", err)
	}
	if err := client.Reconcile(ctx, owner, desired); err != nil {
		t.Fatal("restore after lost mappings", err)
	}
	if err := client.Reconcile(ctx, owner, desired); err != nil {
		t.Fatal("idempotent reconciliation", err)
	}
}

func TestRejectedRuleDoesNotBlockOtherMockNamespaces(t *testing.T) {
	t.Setenv("VMCP_EGRESS_ALLOW_CIDRS", "127.0.0.0/8")
	owner := "owner"
	first := mockupstream.Config{Namespace: mockupstream.Namespace(owner, "first")}
	second := mockupstream.Config{Namespace: mockupstream.Namespace(owner, "second")}
	desired := append(mockupstream.Mappings(owner, first, []mockupstream.Operation{{Name: "bad", Method: "GET", Path: "/", Enabled: true, Rules: []mockupstream.Rule{rule()}}}), mockupstream.Mappings(owner, second, []mockupstream.Operation{{Name: "good", Method: "GET", Path: "/", Enabled: true, Rules: []mockupstream.Rule{rule()}}})...)
	good := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			_, _ = w.Write([]byte(`{"mappings":[]}`))
			return
		}
		var mapping mockupstream.Mapping
		_ = json.NewDecoder(r.Body).Decode(&mapping)
		if strings.HasPrefix(mapping.Name, "bad") {
			w.WriteHeader(422)
			_, _ = w.Write([]byte(`{"errors":[{"title":"Invalid matcher","source":{"pointer":"/request/bodyPatterns"},"detail":"Unsupported matcher"}]}`))
			return
		}
		good = true
		w.WriteHeader(201)
	}))
	defer server.Close()
	err := mockupstream.NewClient(server.URL).Reconcile(context.Background(), owner, desired)
	if err == nil || !strings.Contains(err.Error(), "/request/bodyPatterns") || !good {
		t.Fatalf("another namespace was blocked: %v %t", err, good)
	}
}

func TestMockEndpointRequiresAnOrigin(t *testing.T) {
	for _, base := range []string{"http://example.com/path", "http://example.com?", "http://example.com/#", "http://user:pass@example.com", "file:///tmp/mock", "http://example.com/%2f"} {
		if mockupstream.ValidateEndpoint(base) == nil {
			t.Fatalf("invalid endpoint accepted: %s", base)
		}
	}
	if err := mockupstream.ValidateEndpoint("https://example.com"); err != nil {
		t.Fatal(err)
	}
}
