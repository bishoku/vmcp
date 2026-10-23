package restapi

import (
	"errors"
	"fmt"
	"strings"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
)

// ExecutionError retains a transport cause for structured operator diagnostics.
// Its public text deliberately excludes the cause, which may contain secrets.
type ExecutionError struct {
	Stage string
	cause error
}

func (e *ExecutionError) Error() string {
	if e.Stage == "tls_configuration" {
		return "REST TLS configuration is invalid"
	}
	return "REST request failed or timed out; operation may have completed"
}

func (e *ExecutionError) Unwrap() error { return e.cause }

// responseSchemaHint identifies a configured schema rule, never response values
// or instance paths (which may contain sensitive dynamic object keys).
func responseSchemaHint(err error) string {
	var validation *jsonschema.ValidationError
	if !errors.As(err, &validation) {
		return ""
	}
	for depth := 0; len(validation.Causes) > 0 && depth < 64; depth++ {
		validation = validation.Causes[0]
	}
	location := "/"
	if _, fragment, ok := strings.Cut(validation.SchemaURL, "#"); ok && fragment != "" {
		location = fragment
	}
	if len(location) > 256 {
		location = location[:256] + "…"
	}
	keywords := validation.ErrorKind.KeywordPath()
	rule := "schema"
	if len(keywords) > 0 {
		rule = keywords[0]
	}
	return fmt.Sprintf("; check configured schema %s (%s)", location, rule)
}
