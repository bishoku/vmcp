package audit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestAuditMaskingMergedRulesAndJSON(t *testing.T) {
	s := DefaultSettings()
	s.Scopes["virtual"] = Capture{true, true}
	s.Rules = []Rule{{"email", `[A-Za-z]+@example\.com`}}
	s.VirtualServers["support"] = []Rule{{"customer", `CUST-[0-9]+`}}
	s.Upstreams["crm"] = []Rule{{"employee", `EMP-[0-9]+`}}
	raw := json.RawMessage(`{"nested":["alice@example.com CUST-123 EMP-45"],"token":"secret","authorization":"Bearer opaque","number":12345}`)
	for _, direction := range []string{"request", "response"} {
		p, err := PreviewPayload(s, "virtual", direction, "support", "crm", raw)
		if err != nil {
			t.Fatal(err)
		}
		if !json.Valid(p.Content) || p.State != "redacted" {
			t.Fatalf("invalid preview: %+v", p)
		}
		for _, secret := range []string{"alice@example.com", "CUST-123", "EMP-45", "opaque", "secret"} {
			if bytes.Contains(p.Content, []byte(secret)) {
				t.Fatalf("leaked %s", secret)
			}
		}
		if len(p.Matches) != 3 {
			t.Fatalf("missing rule provenance: %+v", p.Matches)
		}
	}
	if !bytes.Contains(raw, []byte("secret")) {
		t.Fatal("input modified")
	}
}
func TestAuditMaskingBudgetsAndInvalidPolicies(t *testing.T) {
	s := DefaultSettings()
	s.Scopes["control"] = Capture{true, true}
	s.MaxPayloadBytes = 256
	p, err := PreviewPayload(s, "control", "response", "", "", json.RawMessage(`{"x":"`+strings.Repeat("a", 300)+`"}`))
	if err != nil || p.State != "omitted_size" || len(p.Content) > 0 {
		t.Fatal(p, err)
	}
	p, err = PreviewPayload(s, "control", "request", "", "", json.RawMessage(`not json`))
	if err != nil || p.State != "omitted_invalid_json" {
		t.Fatal(p, err)
	}
	p, err = PreviewPayload(s, "virtual", "request", "", "", json.RawMessage(`{"token":"never"}`))
	if err != nil || p.State != "disabled" || len(p.Content) > 0 {
		t.Fatal(p, err)
	}
	for _, pattern := range []string{"(", "a*", "(?=secret)"} {
		s.Rules = []Rule{{"bad", pattern}}
		if s.Validate() == nil {
			t.Fatalf("accepted %s", pattern)
		}
	}
}
func TestAuditMaskingOverlapAndNumbers(t *testing.T) {
	s := DefaultSettings()
	s.Scopes["virtual"] = Capture{true, true}
	s.Rules = []Rule{{"first", `123`}, {"overlap", `234`}, {"numeric", `9999`}}
	p, err := PreviewPayload(s, "virtual", "request", "", "", json.RawMessage(`{"x":"1234","n":9999}`))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(p.Content), "1234") || strings.Contains(string(p.Content), "9999") {
		t.Fatal(string(p.Content))
	}
	if !json.Valid(p.Content) {
		t.Fatal("broken JSON")
	}
}

func TestAuditMaskingEmbeddedJSON(t *testing.T) {
	s := DefaultSettings()
	s.Scopes["virtual"] = Capture{Response: true}
	for _, embedded := range []string{
		`{"result":{"token":"opaque-sensitive-value","id":9007199254740993}}`,
		`[{"password":"opaque-sensitive-value","id":9007199254740993}]`,
		`{"nested":"{\"clientSecret\":\"opaque-sensitive-value\"}","id":9007199254740993}`,
	} {
		raw, _ := json.Marshal(map[string]any{
			"structuredContent": map[string]any{"token": "opaque-sensitive-value"},
			"content":           []any{map[string]string{"type": "text", "text": embedded}},
		})
		p, err := PreviewPayload(s, "virtual", "response", "", "", raw)
		if err != nil || p.State != "redacted" || bytes.Contains(p.Content, []byte("opaque-sensitive-value")) {
			t.Fatalf("embedded secret survived: %s (%v)", p.Content, err)
		}
		var result struct {
			Content []struct{ Text string } `json:"content"`
		}
		if err := json.Unmarshal(p.Content, &result); err != nil {
			t.Fatal(err)
		}
		if len(result.Content) != 1 || !json.Valid([]byte(result.Content[0].Text)) || !strings.Contains(result.Content[0].Text, "9007199254740993") {
			t.Fatalf("string JSON or integer precision lost: %+v", result)
		}
		if !bytes.Contains(raw, []byte("opaque-sensitive-value")) {
			t.Fatal("caller payload modified")
		}
	}
}

func TestAuditMaskingEmbeddedJSONRulesAndLimits(t *testing.T) {
	s := DefaultSettings()
	s.Scopes["virtual"] = Capture{Response: true}
	// Matches across JSON syntax must survive structural decoding.
	s.Rules = []Rule{{"pair", `"left":1, "right":2`}}
	raw, _ := json.Marshal(map[string]string{"text": `{"left":1, "right":2}`})
	p, err := PreviewPayload(s, "virtual", "response", "", "", raw)
	if err != nil || string(p.Content) != `{"text":"[REDACTED]"}` || len(p.Matches) != 1 {
		t.Fatal(string(p.Content), p.Matches, err)
	}
	s.Rules = nil
	for _, embedded := range []string{
		strings.Repeat("[", 65) + `{"token":"secret"}` + strings.Repeat("]", 65),
		"[" + strings.Repeat("0,", 100001) + "0]",
	} {
		raw, _ = json.Marshal(map[string]string{"text": embedded})
		p, err = PreviewPayload(s, "virtual", "response", "", "", raw)
		if err == nil || len(p.Content) != 0 {
			t.Fatal("embedded JSON bypassed traversal budget")
		}
	}
}

func BenchmarkAuditMasking(b *testing.B) {
	for _, size := range []int{1024, 65536, 1048576} {
		b.Run(fmt.Sprint(size), func(b *testing.B) {
			s := DefaultSettings()
			s.MaxPayloadBytes = 1 << 20
			s.Scopes["virtual"] = Capture{true, true}
			s.Rules = []Rule{{"email", `[A-Za-z]+@example\.com`}, {"customer", `CUST-[0-9]+`}}
			raw := json.RawMessage(`{"text":"a@example.com CUST-123 ` + strings.Repeat("x", size-60) + `"}`)
			b.SetBytes(int64(len(raw)))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := PreviewPayload(s, "virtual", "request", "", "", raw); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
