package restapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bishoku/vmcp/core/internal/netpolicy"
	"github.com/bishoku/vmcp/core/internal/tlsconf"
)

func TestExecutionRetainsEgressReasonWithoutExposingRequestSecrets(t *testing.T) {
	t.Setenv("VMCP_EGRESS_ALLOW_HOSTS", "internal.example")
	t.Setenv("VMCP_EGRESS_ALLOW_CIDRS", "")
	config := baseConfig()
	compiled := compile(t, "https://httpbin.org", config)
	result, err := compiled.Execute(context.Background(), "https://httpbin.org", "lookup",
		json.RawMessage(`{"id":123,"q":"secret-query"}`), map[string]string{"X-Key": "secret-header"},
		tlsconf.Settings{}, Identity{}, nil)
	var execution *ExecutionError
	if result != nil || !errors.As(err, &execution) || execution.Stage != "request" || !errors.Is(err, netpolicy.ErrHostNotAllowed) {
		t.Fatalf("missing structured error: %v", err)
	}
	reason, detail := netpolicy.Diagnostic(err)
	if reason != "egress_host_not_allowed" || strings.Contains(err.Error()+reason+detail, "secret") || strings.Contains(err.Error(), "httpbin") {
		t.Fatalf("unsafe or incorrect error: %v, %s, %s", err, reason, detail)
	}
}

func TestResponseSchemaHintUsesSchemaLocationWithoutResponseValues(t *testing.T) {
	schema, err := compileSchema(json.RawMessage(`{"type":"object","properties":{"items":{"type":"array","items":{"type":"object","properties":{"quantity":{"type":"integer"}}}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	err = schema.Validate(map[string]any{"items": []any{map[string]any{"quantity": "private-customer-value"}}})
	if err == nil {
		t.Fatal("expected response validation failure")
	}
	hint := responseSchemaHint(err)
	if !strings.Contains(hint, "/properties/items/items/properties/quantity") || !strings.Contains(hint, "type") || strings.Contains(hint, "private-customer-value") {
		t.Fatalf("invalid hint: %s", hint)
	}
	// Dynamic object keys come from response data and must not be exposed.
	schema, err = compileSchema(json.RawMessage(`{"type":"object","additionalProperties":{"type":"integer"}}`))
	if err != nil {
		t.Fatal(err)
	}
	hint = responseSchemaHint(schema.Validate(map[string]any{"private-account-id": "secret"}))
	if strings.Contains(hint, "private-account-id") || strings.Contains(hint, "secret") {
		t.Fatalf("response data leaked: %s", hint)
	}
}

func TestRuntimeResponseMismatchReturnsSafeSchemaHint(t *testing.T) {
	t.Setenv("VMCP_EGRESS_ALLOW_CIDRS", "127.0.0.0/8")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"private-customer-value"}`))
	}))
	defer server.Close()
	prepared := compile(t, server.URL, baseConfig())
	raw, err := prepared.Execute(context.Background(), server.URL, "lookup", json.RawMessage(`{"id":1}`), nil, tlsconf.Settings{}, Identity{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		IsError bool `json:"isError"`
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err = json.Unmarshal(raw, &result); err != nil || !result.IsError || len(result.Content) != 1 {
		t.Fatalf("unexpected response %s %v", raw, err)
	}
	message := result.Content[0].Text
	if !strings.Contains(message, "/properties/id") || !strings.Contains(message, "(type)") || !strings.Contains(message, "operation may have completed") || strings.Contains(message, "private-customer-value") {
		t.Fatal(message)
	}
}
