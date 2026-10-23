package restapi

import (
	"errors"
	"fmt"
)

// Field paths contain literal property names. Arrays are traversed implicitly,
// preserving every element; no wildcard or dotted-name parsing is involved.
type projectionNode struct {
	whole    bool
	children map[string]*projectionNode
}

func prepareProjection(schema any, fields []string, paths [][]string) (*projectionNode, any, error) {
	if len(fields) > 0 && len(paths) > 0 {
		return nil, nil, errors.New("responseFields and responseFieldPaths cannot both be set")
	}
	if len(fields) == 0 && len(paths) == 0 {
		return nil, schema, nil
	}
	legacy := len(fields) > 0
	if legacy {
		root, ok := schema.(map[string]any)
		if !ok || root["type"] != "object" {
			return nil, nil, errors.New("response field selection requires an object response schema")
		}
		for _, field := range fields {
			paths = append(paths, []string{field})
		}
	} else if len(paths) > 256 {
		return nil, nil, errors.New("configure at most 256 response field paths")
	}
	root := &projectionNode{children: map[string]*projectionNode{}}
	for _, path := range paths {
		if len(path) == 0 || len(path) > 32 {
			return nil, nil, errors.New("response field paths require between 1 and 32 property names")
		}
		current := root
		for _, name := range path {
			if !legacy && len(name) > 1024 {
				return nil, nil, errors.New("response path property names must not exceed 1024 bytes")
			}
			if current.whole {
				return nil, nil, errors.New("response field paths must not overlap")
			}
			if current.children[name] == nil {
				current.children[name] = &projectionNode{children: map[string]*projectionNode{}}
			}
			current = current.children[name]
		}
		if current.whole || len(current.children) > 0 {
			return nil, nil, errors.New("response field paths must be unique and must not overlap")
		}
		current.whole = true
	}
	projected, err := projectSchema(schema, root, 0)
	return root, projected, err
}

func containerType(schema map[string]any) string {
	if typ, ok := schema["type"].(string); ok {
		return typ
	}
	if types, ok := schema["type"].([]any); ok && len(types) == 2 {
		var base string
		nullable := false
		for _, typ := range types {
			if typ == "null" {
				nullable = true
			} else if name, ok := typ.(string); ok {
				base = name
			}
		}
		if nullable {
			return base
		}
	}
	return ""
}

func projectSchema(value any, selection *projectionNode, depth int) (any, error) {
	if selection.whole {
		return value, nil
	}
	if depth > 64 {
		return nil, errors.New("response projection nesting exceeds 64 levels")
	}
	source, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("response field path must traverse declared object properties or homogeneous array items")
	}
	for _, key := range []string{"allOf", "anyOf", "oneOf", "not", "if", "then", "else", "prefixItems", "dependentSchemas", "dependencies"} {
		if _, exists := source[key]; exists {
			return nil, fmt.Errorf("cannot select inside a schema containing %s; select the whole field instead", key)
		}
	}
	output := map[string]any{"type": source["type"]}
	for _, key := range []string{"title", "description"} {
		if value, exists := source[key]; exists {
			output[key] = value
		}
	}
	switch containerType(source) {
	case "array":
		items, exists := source["items"]
		if !exists {
			return nil, errors.New("response array selection requires a declared items schema")
		}
		projected, err := projectSchema(items, selection, depth+1)
		if err != nil {
			return nil, err
		}
		output["items"] = projected
		// Pruning object fields can invalidate uniqueItems, contains, or other
		// value-dependent constraints. Only retain unchanged array length bounds.
		for _, key := range []string{"minItems", "maxItems"} {
			if value, exists := source[key]; exists {
				output[key] = value
			}
		}
		return output, nil
	case "object":
		properties, _ := source["properties"].(map[string]any)
		selected := map[string]any{}
		for name, node := range selection.children {
			child, exists := properties[name]
			if !exists {
				return nil, errors.New("response field paths must reference declared schema properties")
			}
			projected, err := projectSchema(child, node, depth+1)
			if err != nil {
				return nil, fmt.Errorf("response property %q: %w", name, err)
			}
			selected[name] = projected
		}
		required := []string{}
		if names, ok := source["required"].([]any); ok {
			for _, name := range names {
				if text, ok := name.(string); ok && selection.children[text] != nil {
					required = append(required, text)
				}
			}
		}
		output["properties"] = selected
		output["required"] = required
		output["additionalProperties"] = false
		return output, nil
	default:
		return nil, errors.New("response field paths must traverse object or array schemas (optionally nullable)")
	}
}

func (selection *projectionNode) apply(value any, depth int) (any, error) {
	if selection == nil || selection.whole || value == nil {
		return value, nil
	}
	if depth > 64 {
		return nil, errors.New("response projection nesting exceeded")
	}
	switch source := value.(type) {
	case []any:
		output := make([]any, len(source))
		for i, element := range source {
			projected, err := selection.apply(element, depth+1)
			if err != nil {
				return nil, err
			}
			output[i] = projected
		}
		return output, nil
	case map[string]any:
		output := map[string]any{}
		for name, node := range selection.children {
			if child, exists := source[name]; exists {
				projected, err := node.apply(child, depth+1)
				if err != nil {
					return nil, err
				}
				output[name] = projected
			}
		}
		return output, nil
	default:
		return nil, errors.New("REST response cannot be projected using the configured field paths")
	}
}
