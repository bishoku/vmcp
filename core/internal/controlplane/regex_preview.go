package controlplane

import (
	"net/http"

	"github.com/bishoku/vmcp/core/internal/apierror"
	"github.com/bishoku/vmcp/core/internal/audit"
	"github.com/bishoku/vmcp/core/internal/masking"
	"github.com/bishoku/vmcp/core/internal/pii"
)

func (s *Server) previewRegex(w http.ResponseWriter, r *http.Request) error {
	w.Header().Set("Cache-Control", "no-store")
	var body struct{ Kind, Name, Pattern, Sample string }
	if err := decode(r, &body); err != nil {
		return err
	}
	invalid := func(msg string) error { return apierror.New(422, "invalid_regex_preview", "Invalid preview", msg) }
	if len(body.Sample) > 8192 {
		return invalid("Test text must fit within 8192 UTF-8 bytes")
	}
	re, err := masking.Compile(body.Pattern)
	if err != nil {
		return invalid(err.Error())
	}
	matches := re.FindAllStringIndex(body.Sample, -1)
	type part struct {
		Text  string `json:"text"`
		Match bool   `json:"match"`
	}
	parts := []part{}
	cursor := 0
	for _, m := range matches {
		if cursor < m[0] {
			parts = append(parts, part{Text: body.Sample[cursor:m[0]]})
		}
		parts = append(parts, part{body.Sample[m[0]:m[1]], true})
		cursor = m[1]
	}
	if cursor < len(body.Sample) {
		parts = append(parts, part{Text: body.Sample[cursor:]})
	}
	var masked string
	switch body.Kind {
	case "pii":
		if body.Name == "" {
			body.Name = "rule"
		}
		rule := pii.Rule{Name: body.Name, Pattern: body.Pattern}
		if msg := pii.Validate(pii.Config{CustomRules: []pii.Rule{rule}})["/customRules/0/name"]; msg != "" {
			return invalid("Rule name " + msg)
		}
		masked = re.ReplaceAllString(body.Sample, "[REDACTED:"+body.Name+"]")
	case "audit":
		spans := make([][2]int, len(matches))
		for i, m := range matches {
			spans[i] = [2]int{m[0], m[1]}
		}
		masked = audit.MaskSpans(body.Sample, spans)
	default:
		return invalid("Choose audit or pii")
	}
	return writeJSON(w, 200, map[string]any{"parts": parts, "masked": masked, "count": len(matches)})
}
