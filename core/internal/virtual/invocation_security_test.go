package virtual

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"github.com/bishoku/vmcp/core/internal/callercontext"
	"github.com/bishoku/vmcp/core/internal/config"
	"github.com/bishoku/vmcp/core/internal/invocation"
	"github.com/bishoku/vmcp/core/internal/mcp"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestInvocationStepUpEnforcedBeforeExecution(t *testing.T) {
	h := &Handler{}
	snap := &config.Snapshot{InvocationSecurity: invocation.Settings{Upstreams: map[string]invocation.UpstreamPolicy{"up": {Rule: invocation.Rule{Identity: &invocation.IdentityPolicy{Source: "oauth_subject"}, StepUp: &invocation.StepUp{AcceptedACR: []string{"mfa"}, MaxAge: 60}}}}}}
	msg, _ := mcp.NewRequest(mcp.NewIntID(1), "tools/call", nil)
	for _, tc := range []struct {
		mode, acr string
		age       int64
		status    int
		allowed   bool
	}{{"agent-key", "mfa", 0, 403, false}, {"oauth", "password", 0, 401, false}, {"oauth", "mfa", 120, 401, false}, {"oauth", "mfa", 0, 200, true}} {
		w := httptest.NewRecorder()
		id := invocation.Identity{Mode: tc.mode, Subject: "alice", ACR: tc.acr, AuthTime: time.Now().Unix() - tc.age}
		ctx := context.WithValue(context.Background(), assuranceKey{}, id)
		got := h.enforceInvocation(ctx, w, msg, snap, config.VirtualServer{Slug: "server"}, Owner{Mode: tc.mode, Subject: "alice"}, "up", "original", []byte(`{}`), nil)
		if got != tc.allowed || w.Code != tc.status {
			t.Fatalf("%+v: allowed=%v status=%d", tc, got, w.Code)
		}
		if tc.status == 401 && w.Header().Get("WWW-Authenticate") == "" {
			t.Fatal("missing challenge")
		}
	}
}

func TestCallerContextUsesVerifiedCustomerNotApplication(t *testing.T) {
	t.Setenv("VMCP_EGRESS_ALLOW_CIDRS", "127.0.0.0/8,::1/128")
	for _, appMode := range []string{"agent-key", "oauth"} {
		t.Run(appMode, func(t *testing.T) {
			for _, scenario := range []string{"valid", "missing", "forged", "weak", "changed_at_consume"} {
				t.Run(scenario, func(t *testing.T) {
					calls, reservations := 0, 0
					server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						calls++
						var req invocation.Request
						if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
							t.Fatal(err)
						}
						if req.Identity.Subject != "chat-application" || req.Identity.Mode != appMode {
							t.Error("application identity changed")
						}
						if req.CallerContext["sessionId"] != "customer-session" || len(req.CallerContext) != 2 {
							t.Error("wrong context disclosure")
						}
						if req.Phase == "check" && req.User != nil {
							t.Error("claimed customer promoted before verification")
						}
						user := &invocation.Identity{Mode: "external", Issuer: "customer-idp", Subject: "alice", ACR: "mfa", AuthTime: time.Now().Unix()}
						if scenario == "weak" {
							user.ACR = "password"
						}
						if scenario == "changed_at_consume" && req.Phase == "consume" {
							user.Subject = "bob"
						}
						_ = json.NewEncoder(w).Encode(invocation.Decision{Status: "allowed", ApprovalID: "consent", ExpiresAt: time.Now().Add(time.Minute), VerifiedUser: user})
					}))
					defer server.Close()
					cert, _ := x509.ParseCertificate(server.TLS.Certificates[0].Certificate[0])
					svc := invocation.Service{URL: server.URL, ApprovalOrigin: server.URL, Token: "test", CAPEM: string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw}))}
					rule := invocation.Rule{Identity: &invocation.IdentityPolicy{Source: "caller_context", Service: "session", Fields: []string{"customerId", "sessionId"}}, StepUp: &invocation.StepUp{AcceptedACR: []string{"mfa"}, MaxAge: 60}}
					snap := &config.Snapshot{InvocationSecurity: invocation.Settings{Upstreams: map[string]invocation.UpstreamPolicy{"up": {Rule: rule, Services: map[string]invocation.Service{"session": svc}}}}, Upstreams: map[string]config.Upstream{"up": {CallerContext: &callercontext.Config{Enabled: true, MetaKey: "app/customer", Fields: []callercontext.Field{{Name: "customerId", Required: true}, {Name: "sessionId", Required: true}}}}}}
					meta := []byte(`{"io.vmcp/operationId":"6b3debc7-f21e-45ac-900d-9acc65efb9c5","app/customer":{"customerId":"alice","sessionId":"customer-session","mfa":"true"}}`)
					if scenario == "missing" || scenario == "forged" {
						meta = []byte(`{"io.vmcp/operationId":"6b3debc7-f21e-45ac-900d-9acc65efb9c5","app/customer":{"customerId":"alice","mfa":"true"}}`)
					}
					h := &Handler{ReserveInvocation: func(context.Context, string, string) error { reservations++; return nil }}
					msg, _ := mcp.NewRequest(mcp.NewIntID(1), "tools/call", nil)
					ctx := context.WithValue(context.Background(), assuranceKey{}, invocation.Identity{Mode: appMode, Subject: "chat-application", ACR: "mfa", AuthTime: time.Now().Unix()})
					allowed := h.enforceInvocation(ctx, httptest.NewRecorder(), msg, snap, config.VirtualServer{}, Owner{Mode: appMode, Subject: "chat-application"}, "up", "tool", []byte(`{}`), meta)
					if allowed != (scenario == "valid") {
						t.Fatalf("allowed=%v", allowed)
					}
					if (scenario == "missing" || scenario == "forged") && (calls != 0 || reservations != 0) {
						t.Fatal("invalid context reached service")
					}
					if scenario == "weak" && reservations != 0 {
						t.Fatal("application MFA substituted for customer MFA")
					}
					if scenario == "valid" && (calls != 2 || reservations != 1) {
						t.Fatal("check/consume missing")
					}
				})
			}
		})
	}
}

func TestStreamingPreflightReturnsHTTPChallengeBeforeDelivery(t *testing.T) {
	rule := invocation.Rule{Identity: &invocation.IdentityPolicy{Source: "oauth_subject"}, StepUp: &invocation.StepUp{AcceptedACR: []string{"2"}, MaxAge: 60}}
	snap := &config.Snapshot{InvocationSecurity: invocation.Settings{Upstreams: map[string]invocation.UpstreamPolicy{"up": {Rule: rule}}}}
	vs := config.VirtualServer{Tools: []config.VirtualTool{{UpstreamID: "up", UpstreamName: "delete", Tool: mcp.Tool{Name: "exposed-delete"}}}}
	msg, _ := mcp.NewRequest(mcp.NewIntID(1), "tools/call", map[string]any{"name": "exposed-delete", "arguments": map[string]any{}})
	ctx := context.WithValue(context.Background(), assuranceKey{}, invocation.Identity{Mode: "oauth", Subject: "alice", ACR: "1", AuthTime: time.Now().Unix()})
	w := httptest.NewRecorder()
	// A nil delivery store also ensures preflight cannot admit work before 401.
	(&Handler{}).streamCall(ctx, w, httptest.NewRequest("POST", "/", nil), msg, vs, &Session{}, snap)
	if w.Code != 401 || w.Header().Get("WWW-Authenticate") == "" {
		t.Fatalf("challenge lost to streaming: %d", w.Code)
	}
}
