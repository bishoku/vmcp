package callercontext

import (
	"net/http"
	"strings"
	"testing"
)

func TestHeaderMappingValidation(t *testing.T) {
	for _, name := range []string{"Authorization", "Cookie", "Mcp-Session-Id", "MCP-Protocol-Version", "Content-Length", "Host", "Connection", "X-Forwarded-For", "X-VMCP-Agent-Id", "bad name", "bad\r\nname"} {
		for _, rule := range []HeaderMapping{{Source: name, Target: "X-Customer"}, {Source: "X-Customer", Target: name}} {
			if ValidateHeaderMappings([]HeaderMapping{rule}, nil, nil, "mcp") == nil {
				t.Errorf("accepted reserved/invalid mapping: %+v", rule)
			}
		}
	}
	rule := HeaderMapping{Source: "X-Caller", Target: "X-Customer"}
	if err := ValidateHeaderMappings([]HeaderMapping{rule}, nil, nil, "mcp"); err != nil {
		t.Fatal(err)
	}
	if ValidateHeaderMappings([]HeaderMapping{rule, {Source: "X-Other", Target: "x-customer"}}, nil, nil, "rest") == nil {
		t.Fatal("duplicate accepted")
	}
	if ValidateHeaderMappings([]HeaderMapping{rule}, []map[string]string{{"x-customer": "static"}}, nil, "mcp") == nil {
		t.Fatal("static collision accepted")
	}
	if ValidateHeaderMappings([]HeaderMapping{rule}, nil, &Config{Enabled: true, Fields: []Field{{Header: "x-customer"}}}, "rest") == nil {
		t.Fatal("context collision accepted")
	}
	if ValidateHeaderMappings(make([]HeaderMapping, 17), nil, nil, "mcp") == nil {
		t.Fatal("unbounded rules")
	}
}
func TestPrepareRequestHeaders(t *testing.T) {
	rules := []HeaderMapping{{Source: "x-caller", Target: "X-Customer", Required: true}, {Source: "X-Optional", Target: "X-Optional"}}
	incoming := http.Header{"X-Caller": {"one", "two"}, "X-Secret": {"private"}}
	out, err := PrepareHeaders(rules, incoming)
	if err != nil || len(out) != 1 || len(out.Values("X-Customer")) != 2 {
		t.Fatal(out, err)
	}
	incoming["X-Caller"][0] = "changed"
	if out.Get("X-Customer") != "one" {
		t.Fatal("input not cloned")
	}
	for _, h := range []http.Header{nil, {"X-Caller": {""}}, {"X-Caller": {"", " "}}, {"X-Caller": {"\r\nsecret"}}, {"X-Caller": {strings.Repeat("x", 8193)}}, {"X-Caller": {"ok"}, "Connection": {"x-caller"}}} {
		if _, err := PrepareHeaders(rules, h); err == nil {
			t.Fatal("invalid input accepted")
		}
	}
	optional, err := PrepareHeaders([]HeaderMapping{{Source: "X-Missing", Target: "X-Target"}}, nil)
	if err != nil || len(optional) != 0 {
		t.Fatal("optional header required")
	}
}
