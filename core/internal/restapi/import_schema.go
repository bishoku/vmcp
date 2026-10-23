package restapi

import (
	"errors"
	"fmt"
	"maps"
	"strings"
)

func (i *importer) schema(value any, direction string, seen map[string]bool, depth int) (any, error) {
	i.nodes++
	if depth > 48 || i.nodes > 100000 {
		return nil, errors.New("expanded schema complexity limit exceeded")
	}
	if b, ok := value.(bool); ok {
		return b, nil
	}
	source := obj(value)
	if source == nil {
		return nil, errors.New("schema must be an object or boolean")
	}
	if ref, ok := source["$ref"]; ok {
		key := text(ref)
		if seen[key] {
			return nil, errors.New("recursive schemas require manual configuration")
		}
		resolved, err := i.reference(source, seen, 0)
		if err != nil {
			return nil, err
		}
		next := maps.Clone(seen)
		next[key] = true
		return i.schema(resolved, direction, next, depth+1)
	}
	out := map[string]any{}
	for key, value := range source {
		switch key {
		case "$id", "$dynamicRef", "$recursiveRef", "discriminator":
			return nil, errors.New("schema identifiers, dynamic references and discriminators require manual configuration")
		case "$schema":
			if text(value) != "https://json-schema.org/draft/2020-12/schema" && text(value) != "https://spec.openapis.org/oas/3.1/dialect/base" {
				return nil, errors.New("unsupported schema dialect")
			}
		case "nullable", "x-nullable": // converted below for Swagger/OpenAPI 3.0 only
			if !i.legacy {
				out[key] = value
			}
		case "example", "examples", "default", "xml", "externalDocs": // avoid copying example/default credentials into tool schemas
		case "properties", "patternProperties", "dependentSchemas", "$defs", "definitions":
			properties := obj(value)
			if properties == nil {
				return nil, errors.New("schema property maps must be objects")
			}
			converted := map[string]any{}
			for name, property := range properties {
				if key == "properties" && direction == "input" && obj(property) != nil {
					resolved, err := i.reference(obj(property), seen, 0)
					if err != nil {
						return nil, err
					}
					if flag(resolved["readOnly"]) {
						continue
					}
				}
				v, err := i.schema(property, direction, seen, depth+1)
				if err != nil {
					return nil, fmt.Errorf("%s.%s: %w", key, name, err)
				}
				if key == "properties" && direction == "input" && flag(obj(v)["readOnly"]) {
					continue
				}
				converted[name] = v
			}
			out[key] = converted
		case "items", "additionalProperties", "additionalItems", "contains", "not", "if", "then", "else", "propertyNames", "unevaluatedProperties", "unevaluatedItems", "contentSchema":
			v, err := i.schema(value, direction, seen, depth+1)
			if err != nil {
				return nil, err
			}
			out[key] = v
		case "allOf", "anyOf", "oneOf", "prefixItems":
			entries, ok := value.([]any)
			if !ok {
				return nil, errors.New("schema composition must be an array")
			}
			converted := []any{}
			for _, v := range entries {
				schema, err := i.schema(v, direction, seen, depth+1)
				if err != nil {
					return nil, err
				}
				converted = append(converted, schema)
			}
			out[key] = converted
		case "format":
			if text(value) == "binary" || text(value) == "byte" {
				return nil, errors.New("binary schemas are not supported")
			}
			out[key] = value
		case "writeOnly":
			if direction == "output" && flag(value) {
				return nil, errors.New("write-only response fields require explicit response filtering before import")
			}
			out[key] = value
		default:
			if !strings.HasPrefix(key, "x-") {
				out[key] = value
			}
		}
	}
	// Read-only request properties are removed with their required entries.
	if direction == "input" && source["properties"] != nil {
		if required, ok := source["required"].([]any); ok {
			kept := []any{}
			for _, v := range required {
				if _, present := obj(out["properties"])[text(v)]; present {
					kept = append(kept, v)
				} else if _, declared := obj(source["properties"])[text(v)]; !declared {
					kept = append(kept, v)
				}
			}
			out["required"] = kept
		}
	}
	if i.legacy { // Swagger 2.0 and OpenAPI 3.0 use the older boolean exclusivity form.
		for _, bound := range []string{"Minimum", "Maximum"} {
			key := "exclusive" + bound
			if b, ok := out[key].(bool); ok {
				delete(out, key)
				if b {
					limitKey := strings.ToLower(bound)
					limit, exists := out[limitKey]
					if !exists {
						return nil, errors.New("exclusive bound requires a minimum/maximum")
					}
					out[key] = limit
					delete(out, limitKey)
				}
			}
		}
	}
	if i.legacy && (flag(source["nullable"]) || flag(source["x-nullable"])) {
		if typ, ok := out["type"].(string); ok {
			out["type"] = []string{typ, "null"}
		}
	}
	i.bytes += len(raw(out))
	if i.bytes > 16<<20 {
		return nil, errors.New("expanded schema size limit exceeded")
	}
	return out, nil
}
