package restapi

import (
	"errors"
	"fmt"
	"slices"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
)

// requestInput builds the public schema and its binding to the HTTP request body.
func requestInput(op Operation, properties map[string]any, required []string, parameters map[string]string) (map[string]any, *jsonschema.Schema, string, error) {
	input := map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}
	if len(op.BodySchema) == 0 && op.BodyArgumentName == "" {
		return input, nil, "", nil
	}
	if op.Method == "GET" {
		return nil, nil, "", errors.New("GET request bodies are not supported")
	}
	body, err := compileSchema(op.BodySchema)
	if err != nil {
		return nil, nil, "", fmt.Errorf("invalid request body schema: %w", err)
	}
	var shape map[string]any
	_ = decodeJSON(op.BodySchema, &shape)
	typ, _ := shape["type"].(string)
	// Nullable scalar unions still bind to one named argument. An object union
	// would require selecting between incompatible argument shapes at call time.
	if types, ok := shape["type"].([]any); ok && len(types) > 0 && !slices.Contains(types, any("object")) && !slices.Contains(types, any("array")) {
		typ = "scalar"
	}
	if typ == "" {
		return nil, nil, "", errors.New("request body schema must declare an unambiguous root type")
	}
	if op.BodyArgumentName != "" && (typ != "array" || !nameRE.MatchString(op.BodyArgumentName)) {
		return nil, nil, "", errors.New("bodyArgumentName requires an array body and a valid argument name")
	}
	conflict := func(name string) error {
		return fmt.Errorf("argument %q conflicts between %s and request body", name, parameters[name])
	}
	if typ != "object" {
		name := op.BodyArgumentName
		if name == "" {
			name = "body"
		}
		if _, ok := parameters[name]; ok {
			return nil, nil, "", conflict(name)
		}
		properties[name] = shape
		input["required"] = append(required, name)
		return input, body, name, nil
	}
	// Constraints on the complete object cannot simply be applied to an object
	// containing extra URL arguments. Reject ambiguous projections at configuration time.
	for key, val := range shape {
		switch key {
		case "type", "properties", "required":
		case "additionalProperties", "title", "description", "$comment", "$schema", "deprecated", "readOnly", "writeOnly", "default", "examples":
			input[key] = val
		default:
			if len(parameters) > 0 {
				return nil, nil, "", fmt.Errorf("root request body keyword %q cannot be combined with path or query parameters", key)
			}
			input[key] = val
		}
	}
	if _, ok := shape["additionalProperties"]; !ok {
		delete(input, "additionalProperties")
	}
	fields, _ := shape["properties"].(map[string]any)
	for name, field := range fields {
		if _, ok := parameters[name]; ok {
			return nil, nil, "", conflict(name)
		}
		properties[name] = field
	}
	names, _ := shape["required"].([]any)
	for _, item := range names {
		name := item.(string)
		if _, ok := parameters[name]; ok {
			return nil, nil, "", conflict(name)
		}
		required = append(required, name)
	}
	input["required"] = required
	var hidden []string
	for _, p := range op.Parameters {
		if p.ExposeToMCP != nil && !*p.ExposeToMCP {
			hidden = append(hidden, p.Name)
		}
	}
	if len(hidden) > 0 {
		input["propertyNames"] = map[string]any{"not": map[string]any{"enum": hidden}}
	}
	return input, body, "", nil
}
