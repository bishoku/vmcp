package restapi

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestEditorSchemaChecks(t *testing.T) {
	for _, tc := range []struct{ schema, sample, issue string }{
		{`{"type":"object","properties":{"id":{"type":"integer"}}}`, `{"id":1}`, ""},
		{`{"type":"object","properties":{"id":{"type":"integer"}}}`, `{"id":null}`, "/id"},
		{`{"type":"integer","minimum":2}`, `1`, "minimum"},
		{`{"type":"integer"}`, "", ""},
		{`{"type":"integer"}`, "null", "/:"},
		{`{"type":"not-a-type"}`, "", "Invalid schema"},
		{`{"$ref":"https://example.invalid"}`, "", "inline schemas"},
	} {
		issues := (SchemaCheck{Schema: json.RawMessage(tc.schema), Sample: json.RawMessage(tc.sample)}).Issues()
		if tc.issue == "" && len(issues) > 0 || tc.issue != "" && (len(issues) == 0 || !strings.Contains(strings.Join(issues, " "), tc.issue)) {
			t.Errorf("%s with %s: %v", tc.schema, tc.sample, issues)
		}
	}
}
