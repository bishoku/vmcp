package pii

import (
	"encoding/json"
	"strings"
	"testing"
)

func mustMasker(t *testing.T, c Config) *Masker {
	t.Helper()
	c.Enabled = true
	m, err := Compile(c)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestBuiltInDetectors(t *testing.T) {
	m := mustMasker(t, Config{})
	cases := []struct{ in, want string }{
		{"mail ada@example.com now", "mail [REDACTED:EMAIL] now"},
		{"card 4111 1111 1111 1111 ok", "card [REDACTED:CREDIT_CARD] ok"},
		{"card 4111-1111-1111-1111", "card [REDACTED:CREDIT_CARD]"},
		{"iban TR33 0006 1005 1978 6457 8413 26.", "iban [REDACTED:IBAN]."},
		{"iban GB82WEST12345698765432", "iban [REDACTED:IBAN]"},
		{"tckn 10000000146", "tckn [REDACTED:TCKN]"},
		{"ssn 123-45-6789", "ssn [REDACTED:US_SSN]"},
		{"call +90 532 123 45 67", "call [REDACTED:PHONE]"},
		{"call 0532 123 45 67", "call [REDACTED:PHONE]"},
		{"call (212) 555-0147", "call [REDACTED:PHONE]"},
		{"call +905321234567", "call [REDACTED:PHONE]"},
	}
	for _, c := range cases {
		if got, n := m.MaskString(c.in); got != c.want || n != 1 {
			t.Errorf("%q -> %q (n=%d), want %q", c.in, got, n, c.want)
		}
	}
}

func TestNoFalsePositives(t *testing.T) {
	m := mustMasker(t, Config{})
	for _, s := range []string{
		"order 1234567890123456 shipped",  // fails Luhn
		"id 12345678901",                  // fails TCKN checksum
		"date 2024-01-15 and 2024-12-31",  // not a phone
		"host 192.168.10.200 up",          // IPv4 is not a phone
		"version 1.2.3, port 8080",        // plain numbers
		"IBAN-like AB12 CDEF but invalid", // fails mod 97
		"ssn 000-12-3456",                 // invalid area
	} {
		if got, n := m.MaskString(s); got != s || n != 0 {
			t.Errorf("%q was altered: %q (n=%d)", s, got, n)
		}
	}
}

func TestSelectedDetectorsAndCustomRules(t *testing.T) {
	m := mustMasker(t, Config{Detectors: []string{"email"}, CustomRules: []Rule{{Name: "employee", Pattern: `EMP-\d{6}`}}})
	got, n := m.MaskString("ada@example.com EMP-123456 4111 1111 1111 1111")
	if got != "[REDACTED:EMAIL] [REDACTED:employee] 4111 1111 1111 1111" || n != 2 {
		t.Fatalf("got %q n=%d", got, n)
	}
}

func TestMaskJSON(t *testing.T) {
	m := mustMasker(t, Config{})
	in := json.RawMessage(`{"content":[{"type":"text","text":"mail ada@example.com"},{"type":"image","data":"ada@example.com","mimeType":"image/png"}],"structuredContent":{"owner":"bob@example.com","count":3},"isError":false}`)
	out, n, err := m.MaskJSON(in)
	if err != nil || n != 2 {
		t.Fatalf("n=%d err=%v out=%s", n, err, out)
	}
	s := string(out)
	if strings.Contains(s, "bob@example.com") || strings.Contains(s, "mail ada@") {
		t.Fatalf("not masked: %s", s)
	}
	if !strings.Contains(s, `"data":"ada@example.com"`) {
		t.Fatalf("binary image payload must be untouched: %s", s)
	}
	// Untouched documents keep their exact bytes.
	same := json.RawMessage(`{"a": "nothing here"}`)
	if out, n, _ := m.MaskJSON(same); n != 0 || string(out) != string(same) {
		t.Fatalf("unchanged doc rewritten: %s", out)
	}
}

func TestValidate(t *testing.T) {
	errs := Validate(Config{Detectors: []string{"bogus"}, CustomRules: []Rule{
		{Name: "ok", Pattern: `x+`}, {Name: "ok", Pattern: `y`}, {Name: "bad name!", Pattern: `z`},
		{Name: "broken", Pattern: `(`}, {Name: "empty", Pattern: `a*`}, {Name: "none", Pattern: ``},
	}})
	for _, k := range []string{"/detectors/0", "/customRules/1/name", "/customRules/2/name", "/customRules/3/pattern", "/customRules/4/pattern", "/customRules/5/pattern"} {
		if errs[k] == "" {
			t.Errorf("missing error at %s (got %v)", k, errs)
		}
	}
	if _, err := Compile(Config{Enabled: false, Detectors: []string{"bogus"}}); err != nil {
		t.Fatal("disabled config must not be compiled")
	}
}
