package controlplane

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRegexPreview(t *testing.T) {
	for _, tc := range []struct {
		kind, pattern, sample, want string
		count                       int
	}{
		{"audit", "x", "🙂xx\nx", "🙂[REDACTED]\n[REDACTED]", 3},
		{"pii", "x", "🙂xx", "🙂[REDACTED:id][REDACTED:id]", 2},
		{"pii", "x", "İstanbul", "İstanbul", 0},
		{"pii", "(?<=a)b", "ab", "", -1},
		{"audit", "x*", "x", "", -1},
		{"audit", "x", strings.Repeat("ü", 4097), "", -1},
	} {
		body, _ := json.Marshal(map[string]string{"kind": tc.kind, "name": "id", "pattern": tc.pattern, "sample": tc.sample})
		w := httptest.NewRecorder()
		err := new(Server).previewRegex(w, httptest.NewRequest("POST", "/api/v1/regex-preview", bytes.NewReader(body)))
		if tc.count < 0 {
			if err == nil {
				t.Fatal("expected validation error")
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		var got struct {
			Masked string
			Count  int
			Parts  []struct {
				Text  string
				Match bool
			}
		}
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		var original strings.Builder
		var marked int
		for _, p := range got.Parts {
			original.WriteString(p.Text)
			if p.Match {
				marked++
				if p.Text != "x" {
					t.Fatalf("unexpected highlight %q", p.Text)
				}
			}
		}
		if original.String() != tc.sample || marked != tc.count || got.Count != tc.count || got.Masked != tc.want {
			t.Fatalf("unexpected result %+v", got)
		}
	}
}
