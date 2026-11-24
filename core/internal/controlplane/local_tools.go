package controlplane

import (
	"encoding/json"
	"github.com/bishoku/vmcp/core/internal/apierror"
	"github.com/bishoku/vmcp/core/internal/localtools"
	"net/http"
)

func (s *Server) previewLocalTool(w http.ResponseWriter, r *http.Request) error {
	var input struct {
		Tool      localtools.Definition `json:"tool"`
		Arguments json.RawMessage       `json:"arguments"`
	}
	if err := decode(r, &input); err != nil {
		return err
	}
	c, err := localtools.Compile(localtools.Config{Tools: []localtools.Definition{input.Tool}})
	if err != nil {
		return apierror.Validation([]apierror.FieldError{{Field: "/tool", Code: apierror.CodeValidationFailed, Message: err.Error()}})
	}
	if input.Arguments == nil {
		return writeJSON(w, 200, map[string]string{"text": ""})
	}
	text, err := c.Render(input.Tool.Name, input.Arguments)
	if err != nil {
		return apierror.Validation([]apierror.FieldError{{Field: "/arguments", Code: apierror.CodeValidationFailed, Message: err.Error()}})
	}
	return writeJSON(w, 200, map[string]string{"text": text})
}
