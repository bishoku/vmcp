package restapi

import (
	"context"
	"encoding/json"
	"errors"
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
