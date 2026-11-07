package invocation

import (
	"encoding/json"
	"testing"
	"time"
)

func TestInheritedPolicyCannotBeWeakened(t *testing.T) {
	parent := Rule{StepUp: &StepUp{[]string{"mfa", "hardware"}, 60}, ApprovalServices: []string{"owner"}, ShareFields: []string{"amount"}}
	child := Rule{StepUp: &StepUp{[]string{"hardware", "password"}, 300}, ApprovalServices: []string{"risk"}, ShareFields: []string{"amount", "secret"}}
	got, err := Merge(parent, child)
	if err != nil {
		t.Fatal(err)
	}
	if got.StepUp.MaxAge != 60 || len(got.StepUp.AcceptedACR) != 1 || got.StepUp.AcceptedACR[0] != "hardware" || len(got.ApprovalServices) != 2 || len(got.ShareFields) != 1 {
		t.Fatalf("weakened: %+v", got)
	}
	if _, err := Merge(parent, Rule{StepUp: &StepUp{[]string{"password"}, 60}}); err == nil {
		t.Fatal("unsatisfiable policy accepted")
	}
}
func TestAssuranceRequiresRealRecentUser(t *testing.T) {
	now := time.Unix(1000, 0)
	p := StepUp{[]string{"mfa"}, 60}
	good := Identity{Mode: "oauth", Subject: "alice", ACR: "mfa", AuthTime: 950}
	if !p.Satisfied(good, now) {
		t.Fatal("valid user rejected")
	}
	for _, id := range []Identity{{Mode: "agent-key", Subject: "alice", ACR: "mfa", AuthTime: 950}, {Mode: "oauth", Subject: "alice", ACR: "mfa", AuthTime: 900}, {Mode: "oauth", Subject: "alice", ACR: "password", AuthTime: 950}, {Mode: "oauth", Subject: "alice", ACR: "mfa", AuthTime: 1100}} {
		if p.Satisfied(id, now) {
			t.Fatal("invalid assurance accepted")
		}
	}
	id := ClaimsIdentity("oauth", "binding", "issuer", "alice", map[string]any{"acr": "mfa", "auth_time": 999.5})
	if p.Satisfied(id, now) {
		t.Fatal("fractional auth_time accepted")
	}
}
func TestDigestExactAmountsAndAllowlistedDetails(t *testing.T) {
	a, details, e := Content([]byte(`{"secret":"hidden","amount":9007199254740993}`), []string{"amount"})
	if e != nil {
		t.Fatal(e)
	}
	b, _, _ := Content([]byte(`{ "amount":9007199254740993, "secret":"hidden" }`), nil)
	c, _, _ := Content([]byte(`{"secret":"hidden","amount":9007199254740992}`), nil)
	if a != b || a == c {
		t.Fatal("digest loses identity or precision")
	}
	raw, _ := json.Marshal(details)
	if string(raw) != `{"amount":9007199254740993}` {
		t.Fatal(string(raw))
	}
}

func TestAmbiguousJSONCannotBeApproved(t *testing.T) {
	for _, raw := range []string{`{"amount":1,"amount":2}`, `{"nested":{"x":1,"x":2}}`, `{} {}`} {
		if _, _, err := Content([]byte(raw), nil); err == nil {
			t.Fatalf("ambiguous JSON accepted: %s", raw)
		}
	}
}

func TestIdentitySourceIsExplicitAndInherited(t *testing.T) {
	parent := Rule{Identity: &IdentityPolicy{Source: "caller_context", Service: "session", Fields: []string{"customerId", "sessionId"}}}
	if got, err := Merge(parent, Rule{}); err != nil || got.Identity.Source != "caller_context" {
		t.Fatal("identity not inherited", err)
	}
	for _, child := range []Rule{{Identity: &IdentityPolicy{Source: "oauth_subject"}}, {Identity: &IdentityPolicy{Source: "caller_context", Service: "other", Fields: []string{"customerId", "sessionId"}}}, {Identity: &IdentityPolicy{Source: "caller_context", Service: "session", Fields: []string{"customerId"}}}} {
		if _, err := Merge(parent, child); err == nil {
			t.Fatal("identity override accepted")
		}
	}
	s := Settings{Upstreams: map[string]UpstreamPolicy{"up": {Rule: Rule{StepUp: &StepUp{AcceptedACR: []string{"2"}, MaxAge: 60}}}}}
	if s.Validate() == nil {
		t.Fatal("implicit identity source accepted")
	}
}
