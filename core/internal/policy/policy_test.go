package policy

import (
	"encoding/json"
	"testing"
)

func TestDecide(t *testing.T) {
	ro := json.RawMessage(`{"readOnlyHint":true}`)
	cases := []struct {
		name string
		c    Config
		tool string
		ann  json.RawMessage
		want bool
	}{
		{"zero allows", Config{}, "a", nil, true},
		{"deny", Config{Default: Deny}, "a", ro, false},
		{"read_only with hint", Config{Default: ReadOnly}, "a", ro, true},
		{"read_only without annotations", Config{Default: ReadOnly}, "a", nil, false},
		{"read_only hint false", Config{Default: ReadOnly}, "a", json.RawMessage(`{"readOnlyHint":false}`), false},
		{"override allows", Config{Default: Deny, Tools: map[string]string{"a": Allow}}, "a", nil, true},
		{"override denies", Config{Tools: map[string]string{"a": Deny}}, "a", ro, false},
	}
	for _, tc := range cases {
		if got, _ := tc.c.Decide(tc.tool, tc.ann); got != tc.want {
			t.Errorf("%s: got %v want %v", tc.name, got, tc.want)
		}
	}
}

func TestValidate(t *testing.T) {
	errs := Validate(Config{Default: "nope", Tools: map[string]string{"a": "x", "ghost": Deny, "a/b": Allow}}, map[string]bool{"a": true, "a/b": true})
	for _, k := range []string{"/default", "/tools/a", "/tools/ghost"} {
		if errs[k] == "" {
			t.Errorf("missing error at %s: %v", k, errs)
		}
	}
	if len(errs) != 3 {
		t.Errorf("unexpected errors: %v", errs)
	}
}
