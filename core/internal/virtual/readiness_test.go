package virtual

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bishoku/vmcp/core/internal/invocation"
	"github.com/bishoku/vmcp/core/internal/mcp"
	"github.com/bishoku/vmcp/core/internal/pii"
	"github.com/bishoku/vmcp/core/internal/telemetry"
)

func TestInvalidSchemaArgumentsNeverReachUpstreamOrApproval(t *testing.T) {
	r := newRig(t)
	snap := r.handler.Snap.Snapshot()
	vs := snap.VirtualServers["support"]
	vs.Tools[0].Tool.InputSchema = json.RawMessage(`{"type":"object","properties":{"count":{"type":"integer"}},"required":["count"]}`)
	snap.VirtualServers["support"] = vs
	snap.InvocationSecurity.Upstreams = map[string]invocation.UpstreamPolicy{"u-gh": {Rule: invocation.Rule{Identity: &invocation.IdentityPolicy{Source: "oauth_subject"}, ApprovalServices: []string{"approval"}}}}
	sid := r.initialize(t)
	_, reply := r.rpc(t, "support", sid, mcp.MethodToolsCall, mcp.CallToolParams{Name: "github__create_issue", Arguments: json.RawMessage(`{"count":"PRIVATE_VALUE"}`)})
	if reply.Error == nil || reply.Error.Code != mcp.CodeInvalidParams || strings.Contains(reply.Error.Message, "PRIVATE_VALUE") {
		t.Fatal(reply)
	}
	r.github.mu.Lock()
	defer r.github.mu.Unlock()
	if r.github.inits != 0 {
		t.Fatal("invalid call connected to upstream")
	}
}

func TestMaskedArgumentsRevalidatedBeforeApproval(t *testing.T) {
	r := newRig(t)
	snap := r.handler.Snap.Snapshot()
	vs := snap.VirtualServers["support"]
	vs.Tools[0].Tool.InputSchema = json.RawMessage(`{"type":"object","properties":{"email":{"const":"person@example.com"}},"required":["email"]}`)
	masker, err := pii.Compile(pii.Config{Enabled: true, MaskArguments: true, Detectors: []string{"email"}})
	if err != nil {
		t.Fatal(err)
	}
	vs.PII = masker
	snap.VirtualServers["support"] = vs
	snap.InvocationSecurity.Upstreams = map[string]invocation.UpstreamPolicy{"u-gh": {Rule: invocation.Rule{Identity: &invocation.IdentityPolicy{Source: "oauth_subject"}, ApprovalServices: []string{"approval"}}}}
	sid := r.initialize(t)
	_, reply := r.rpc(t, "support", sid, mcp.MethodToolsCall, mcp.CallToolParams{Name: "github__create_issue", Arguments: json.RawMessage(`{"email":"person@example.com"}`)})
	if reply.Error == nil || reply.Error.Code != mcp.CodeInvalidParams {
		t.Fatal(reply)
	}
	r.github.mu.Lock()
	defer r.github.mu.Unlock()
	if r.github.inits != 0 {
		t.Fatal("masked invalid arguments reached upstream")
	}
}

func TestApprovalBoundCallNeverRetriesDespiteOperatorPreference(t *testing.T) {
	r := newRig(t)
	metrics := telemetry.New()
	r.handler.Metrics = metrics
	failed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(503) }))
	defer failed.Close()
	snap := r.handler.Snap.Snapshot()
	up := snap.Upstreams["u-gh"]
	up.URL = failed.URL
	sid := r.initialize(t)
	ctx := context.WithValue(context.Background(), retryInvocationKey{}, "once")
	ctx = context.WithValue(ctx, approvedInvocationKey{}, true)
	_, err := r.handler.callResilient(ctx, r.handler.Sessions.Get(sid), up, snap.VirtualServers["support"].Tools[0], nil)
	if err == nil {
		t.Fatal("failed upstream succeeded")
	}
	out := httptest.NewRecorder()
	metrics.ServeHTTP(out, httptest.NewRequest("GET", "/metrics", nil))
	if !strings.Contains(out.Body.String(), `vmcp_upstream_requests_total{upstream="github",method="tools/call",tool="create_issue",outcome="error"} 1`) {
		t.Fatal(out.Body.String())
	}
}
