package securitydemo

import (
	"bytes"
	"encoding/json"
	"github.com/bishoku/vmcp/core/internal/invocation"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestApprovalBindsOperationAndConsumesOnce(t *testing.T) {
	s := &Server{Secret: "test-secret", Origin: "https://approval.test", operations: map[string]*entry{}}
	req := invocation.Request{IdentitySource: "oauth_subject", User: &invocation.Identity{Subject: "alice", Issuer: "issuer"}, Requirements: invocation.Requirements{Approval: true}, Version: "1", Phase: "check", OperationID: "op", Identity: invocation.Identity{Subject: "alice", Issuer: "issuer"}, Digest: "original", Tool: "delete", PolicyRevision: 1}
	call := func(r invocation.Request) invocation.Decision {
		t.Helper()
		raw, _ := json.Marshal(r)
		request := httptest.NewRequest("POST", "/check", bytes.NewReader(raw))
		request.Header.Set("Authorization", "Bearer test-secret")
		w := httptest.NewRecorder()
		s.check(w, request)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		var d invocation.Decision
		if json.Unmarshal(w.Body.Bytes(), &d) != nil {
			t.Fatal("invalid JSON")
		}
		return d
	}
	if call(req).Status != "pending" {
		t.Fatal("must start pending")
	}
	s.operations["op"].Decision.Status = "allowed"
	changed := req
	changed.Digest = "modified"
	if call(changed).Status != "denied" {
		t.Fatal("changed arguments accepted")
	}
	changed = req
	other := *req.User
	other.Subject = "bob"
	changed.User = &other
	if call(changed).Status != "denied" {
		t.Fatal("wrong user accepted")
	}
	changed = req
	changed.PolicyRevision = 2
	if call(changed).Status != "denied" {
		t.Fatal("changed policy accepted")
	}
	req.Phase = "consume"
	if call(req).Status != "allowed" {
		t.Fatal("approved request rejected")
	}
	if call(req).Status != "denied" {
		t.Fatal("replay accepted")
	}
	req.OperationID = "expired"
	req.Phase = "check"
	call(req)
	s.operations["expired"].Decision.ExpiresAt = time.Now().Add(-time.Second)
	if call(req).Status != "denied" {
		t.Fatal("expired approval accepted")
	}
}

func TestSharedApplicationCustomerSessionsAreIndependent(t *testing.T) {
	s := &Server{Secret: "test-secret", Origin: "https://approval.test", operations: map[string]*entry{}, customers: map[string]*customerSession{
		"alice-session": {User: invocation.Identity{Issuer: "idp", Subject: "alice", ACR: "1", AuthTime: time.Now().Unix()}, CustomerID: "alice", ExpiresAt: time.Now().Add(time.Minute)},
		"bob-session":   {User: invocation.Identity{Issuer: "idp", Subject: "bob", ACR: "2", AuthTime: time.Now().Unix()}, CustomerID: "bob", ExpiresAt: time.Now().Add(time.Minute)},
	}}
	req := invocation.Request{Version: "1", Phase: "check", OperationID: "shared", IdentitySource: "caller_context", Identity: invocation.Identity{Mode: "agent-key", CredentialID: "shared-application"}, CallerContext: map[string]string{"customerId": "alice", "sessionId": "alice-session"}, Requirements: invocation.Requirements{Approval: true, StepUp: &invocation.StepUp{AcceptedACR: []string{"2"}, MaxAge: 60}}}
	call := func() invocation.Decision {
		t.Helper()
		raw, _ := json.Marshal(req)
		r := httptest.NewRequest("POST", "/check", bytes.NewReader(raw))
		r.Header.Set("Authorization", "Bearer test-secret")
		w := httptest.NewRecorder()
		s.check(w, r)
		var result invocation.Decision
		if json.Unmarshal(w.Body.Bytes(), &result) != nil {
			t.Fatal(w.Body.String())
		}
		return result
	}
	if d := call(); d.Status != "step_up_required" || strings.Contains(d.URL, "alice-session") {
		t.Fatal("invalid challenge", d.Status)
	}
	req.CallerContext["customerId"] = "bob"
	if call().Status != "denied" {
		t.Fatal("customer/session mismatch accepted")
	}
	req.CallerContext["customerId"] = "alice"
	s.customers["alice-session"].User.ACR = "2"
	if d := call(); d.Status != "pending" || d.VerifiedUser.Subject != "alice" {
		t.Fatal("verified customer missing")
	}
	s.operations["shared"].Decision.Status = "allowed"
	req.CallerContext = map[string]string{"customerId": "bob", "sessionId": "bob-session"}
	if call().Status != "denied" {
		t.Fatal("another customer reused approval under same agent key")
	}
	req.CallerContext = map[string]string{"customerId": "alice", "sessionId": "alice-session"}
	req.Phase = "consume"
	if call().Status != "allowed" {
		t.Fatal("correct session rejected")
	}
	if call().Status != "denied" {
		t.Fatal("consumed operation reused")
	}
}
