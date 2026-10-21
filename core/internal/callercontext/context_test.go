package callercontext

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func testConfig() *Config {
	return &Config{Enabled: true, MetaKey: "bank/context", Fields: []Field{{Name: "token", Header: "Authorization", Bearer: true, Required: true}, {Name: "session", Header: "X-Chat-Session"}}}
}
func TestValidation(t *testing.T) {
	for _, tc := range []struct {
		name    string
		change  func(*Config)
		static  []map[string]string
		managed bool
		kind    string
	}{
		{name: "reserved", change: func(c *Config) { c.Fields[0].Header = "X-VMCP-Agent-Id" }},
		{name: "transport", change: func(c *Config) { c.Fields[0].Header = "Host" }},
		{name: "duplicate", change: func(c *Config) { c.Fields[1].Header = "authorization" }},
		{name: "static", static: []map[string]string{{"authorization": "secret"}}},
		{name: "managed", managed: true},
		{name: "namespace", change: func(c *Config) { c.MetaKey = "progressToken" }},
		{name: "mcp headers", kind: "mcp"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := testConfig()
			if tc.change != nil {
				tc.change(c)
			}
			kind := tc.kind
			if kind == "" {
				kind = "rest"
			}
			if c.Validate(kind, tc.static, tc.managed) == nil {
				t.Fatal("accepted invalid config")
			}
		})
	}
	if err := testConfig().Validate("rest", nil, false); err != nil {
		t.Fatal(err)
	}
}
func TestPrepare(t *testing.T) {
	c := testConfig()
	for _, raw := range []string{`{}`, `{"bank/context":null}`, `{"bank/context":{"token":42}}`, `{"bank/context":{"token":null}}`, `{"bank/context":{"token":""}}`, `{"bank/context":{"token":"secret\r\nInjected: value"}}`, strings.Repeat(" ", MaxBytes+1)} {
		_, _, err := c.Prepare(json.RawMessage(raw), "rest")
		if err == nil {
			t.Fatalf("accepted %q", raw)
		}
		if strings.Contains(err.Error(), "secret") {
			t.Fatal("value in error")
		}
	}
	meta, h, err := c.Prepare(json.RawMessage(`{"progressToken":"p","other":"no","bank/context":{"token":"t","session":"s","ignored":"no"}}`), "rest")
	if err != nil || h.Get("Authorization") != "Bearer t" || h.Get("X-Chat-Session") != "s" || strings.Contains(string(meta), "ignored") || strings.Contains(string(meta), "progressToken") {
		t.Fatalf("%s %v %v", meta, h, err)
	}
	c.Enabled = false
	if m, h, err := c.Prepare(json.RawMessage(`{"bank/context":{"token":false}}`), "rest"); err != nil || m != nil || h != nil {
		t.Fatal("disabled config processed metadata")
	}
	c.Enabled = true
	c.Fields[0].Required = false
	if _, h, err := c.Prepare(nil, "rest"); err != nil || len(h) != 0 {
		t.Fatal("optional context required")
	}
}
func TestConcurrentHeadersAreRequestScoped(t *testing.T) {
	c := testConfig()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			token := fmt.Sprintf("token-%d", i)
			raw, _ := json.Marshal(map[string]any{"bank/context": map[string]string{"token": token}})
			_, h, err := c.Prepare(raw, "rest")
			if err != nil {
				t.Error(err)
				return
			}
			ctx := WithHeaders(context.Background(), h)
			h.Set("Authorization", "mutated")
			req := httptest.NewRequest("GET", "https://example.com", nil)
			ApplyHeaders(ctx, req)
			if req.Header.Get("Authorization") != "Bearer "+token {
				t.Error("context mixed")
			}
		}(i)
	}
	wg.Wait()
}
