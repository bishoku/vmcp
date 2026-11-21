package restapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/bishoku/vmcp/core/internal/tlsconf"
)

func TestTextResponseBoundaries(t *testing.T) {
	t.Setenv("VMCP_EGRESS_ALLOW_CIDRS", "127.0.0.0/8")
	for _, tc := range []struct {
		name, body string
		status     int
		wantError  string
	}{
		{"exact limit", strings.Repeat("z", MaxResponseBytes), 200, ""},
		{"over limit", strings.Repeat("z", MaxResponseBytes+1), 200, "exceeded"},
		{"invalid UTF-8", string([]byte{0xff}), 200, "invalid UTF-8"},
		{"empty", "", 200, ""},
		{"no content", "", 204, ""},
		{"JSON-looking text", `{"x":42}`, 200, ""},
		{"upstream error", "private upstream error detail", 400, "HTTP 400"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Header.Get("Accept") != "text/plain" || r.Header.Get("Content-Type") != "application/json" {
					t.Error(r.Header)
				}
				w.WriteHeader(tc.status)
				io.WriteString(w, tc.body)
			}))
			defer srv.Close()
			cfg := baseConfig()
			cfg.Operations = []Operation{{Name: "report", Enabled: true, Method: "POST", Path: "/report", ResponseFormat: "text", BodySchema: json.RawMessage(`{"type":"object"}`)}}
			compiled := compile(t, srv.URL, cfg)
			out, err := compiled.Execute(context.Background(), srv.URL, "report", json.RawMessage(`{}`), nil, tlsconf.Settings{}, Identity{}, nil)
			if err != nil || calls.Load() != 1 {
				t.Fatal(err, calls.Load())
			}
			var result struct {
				StructuredContent *struct{ Result *string }
				IsError           bool
				Content           []struct{ Text string }
			}
			if err := json.Unmarshal(out, &result); err != nil {
				t.Fatal(err)
			}
			if tc.wantError != "" {
				if !result.IsError || result.StructuredContent != nil || len(result.Content) != 1 || !strings.Contains(result.Content[0].Text, tc.wantError) || strings.Contains(result.Content[0].Text, "private upstream") {
					t.Fatal("wrong error response")
				}
			} else if result.IsError || result.StructuredContent == nil || result.StructuredContent.Result == nil || *result.StructuredContent.Result != tc.body {
				t.Fatal("text result mismatch")
			}
		})
	}
}

func TestResponseFormatValidation(t *testing.T) {
	for _, op := range []Operation{
		{ResponseFormat: "xml"},
		{ResponseFormat: "text", ResponseSchema: json.RawMessage(`{"type":"object"}`)},
		{ResponseFormat: "text", ResponseFields: []string{"id"}},
		{ResponseFormat: "text", ResponseFieldPaths: [][]string{{"id"}}},
	} {
		op.Name = "get"
		op.Method = "GET"
		op.Path = "/"
		cfg := baseConfig()
		cfg.Operations = []Operation{op}
		if _, err := Compile("https://example.com", cfg, nil); err == nil {
			t.Fatal(op)
		}
	}
	cfg := baseConfig()
	cfg.Operations[0].ResponseFormat = "json"
	compile(t, "https://example.com", cfg)
}
