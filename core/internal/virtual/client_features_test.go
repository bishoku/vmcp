package virtual

import (
	"encoding/json"
	"github.com/bishoku/vmcp/core/internal/config"
	"github.com/bishoku/vmcp/core/internal/mcp"
	"strings"
	"testing"
)

func TestClientFeatureNegotiationAndBudgets(t *testing.T) {
	session := &Session{clientCapabilities: json.RawMessage(`{"sampling":{},"roots":{"listChanged":true},"elicitation":{},"unknown":{}}`)}
	vs := config.VirtualServer{}
	for _, method := range []string{"sampling/createMessage", "roots/list", "elicitation/create"} {
		if featureAllowed(method, vs, session) {
			t.Fatal("default policy allowed client feature")
		}
	}
	vs.ClientFeatures = config.ClientFeatures{Sampling: true, Roots: true, Elicitation: true}
	for _, method := range []string{"sampling/createMessage", "roots/list", "elicitation/create"} {
		if !featureAllowed(method, vs, session) {
			t.Fatal("negotiated client feature denied")
		}
	}
	session.clientCapabilities = json.RawMessage(`{}`)
	if featureAllowed("sampling/createMessage", vs, session) {
		t.Fatal("incapable client received sampling")
	}
	if strings.Contains(string(vs.ClientFeatures.Negotiated(json.RawMessage(`{"unknown":{}}`))), "unknown") {
		t.Fatal("unsupported capability forwarded")
	}
	budget := &featureBudget{}
	if !budget.reserve(4096) || !budget.reserve(4096) || budget.reserve(1) {
		t.Fatal("sampling token budget not enforced")
	}
	budget = &featureBudget{}
	for i := 0; i < 8; i++ {
		if !budget.reserve(0) {
			t.Fatal("premature request limit")
		}
	}
	if budget.reserve(0) {
		t.Fatal("client request flood accepted")
	}
	for _, test := range []struct{ method, params string }{{"sampling/createMessage", `{"maxTokens":4097,"messages":[{"role":"user","content":{"type":"text","text":"x"}}]}`}, {"sampling/createMessage", `{"maxTokens":10,"includeContext":"allServers","messages":[{"role":"user","content":{"type":"text","text":"x"}}]}`}, {"elicitation/create", `{"mode":"url","url":"https://example.com/consent"}`}, {"elicitation/create", `{"message":"x","requestedSchema":{"type":"object","$ref":"https://127.0.0.1/secret","properties":{}}}`}, {"elicitation/create", `{"message":"x","requestedSchema":{"type":"object","properties":{"password":{"type":"string","format":"password"}}}}`}} {
		msg, _ := mcp.NewRequest(mcp.NewIntID(1), test.method, json.RawMessage(test.params))
		if _, err := validateFeatureRequest(msg); err == nil {
			t.Fatalf("invalid request accepted: %s", test.params)
		}
	}
	for _, test := range []struct{ method, result string }{{"roots/list", `{"roots":[{"uri":"https://example.com/"}]}`}, {"sampling/createMessage", `{"role":"assistant","content":{"type":"text","text":"x"}}`}, {"elicitation/create", `{"action":"approve"}`}, {"elicitation/create", `{"action":"decline","content":{"secret":"x"}}`}} {
		response, _ := mcp.NewResult(mcp.NewIntID(1), json.RawMessage(test.result))
		if validateFeatureResult(test.method, response) == nil {
			t.Fatal("invalid client result accepted")
		}
	}
}

func TestElicitationCannotBypassRequestedForm(t *testing.T) {
	request := json.RawMessage(`{"requestedSchema":{"type":"object","properties":{"choice":{"type":"string","enum":["yes","no"]},"age":{"type":"integer","minimum":18}},"required":["choice"]}}`)
	for _, raw := range []string{`{"action":"accept","content":{}}`, `{"action":"accept","content":{"choice":"maybe"}}`, `{"action":"accept","content":{"choice":"yes","age":17}}`, `{"action":"accept","content":{"choice":"yes","unknown":"x"}}`} {
		response, _ := mcp.NewResult(mcp.NewIntID(1), json.RawMessage(raw))
		if validateElicitationContent(request, response) == nil {
			t.Fatal("invalid accepted form forwarded")
		}
	}
	response, _ := mcp.NewResult(mcp.NewIntID(1), map[string]string{"action": "decline"})
	if err := validateElicitationContent(request, response); err != nil {
		t.Fatal("decline rejected")
	}
}
