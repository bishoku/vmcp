package restapi

import (
	"encoding/json"
	"errors"
	"fmt"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
)

// SchemaCheck validates one transient editor schema. Absent sample skips instance validation;
// the literal JSON null is a supplied sample.
type SchemaCheck struct {
	Schema json.RawMessage `json:"schema"`
	Sample json.RawMessage `json:"sample,omitempty"`
}

func (c SchemaCheck) Issues() []string {
	if len(c.Schema) == 0 {
		return []string{"A schema is required."}
	}
	compiled, err := compileSchema(c.Schema)
	if err != nil {
		return []string{fmt.Sprintf("Invalid schema: %v", err)}
	}
	if len(c.Sample) > 0 {
		var value any
		if err := decodeJSON(c.Sample, &value); err != nil {
			return []string{"Invalid JSON sample."}
		}
		if err := compiled.Validate(value); err != nil {
			var validation *jsonschema.ValidationError
			if !errors.As(err, &validation) {
				return []string{err.Error()}
			}
			result := []string{}
			for _, issue := range validation.BasicOutput().Errors {
				if issue.Error == nil {
					continue
				}
				path := issue.InstanceLocation
				if path == "" {
					path = "/"
				}
				result = append(result, fmt.Sprintf("%s: %s", path, issue.Error.String()))
				if len(result) == 8 {
					break
				}
			}
			if len(result) > 0 {
				return result
			}
			return []string{err.Error()}
		}
	}
	return []string{}
}
