package restapi

import (
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

var importNameCharacters = regexp.MustCompile(`[^A-Za-z0-9_.-]+`)

func (i *importer) operation(path, method string, pathItem, source map[string]any, warnings *[]string) (Operation, error) {
	out := Operation{Method: strings.ToUpper(method), Path: path, Enabled: true, Parameters: []Parameter{}, ReadOnly: false}
	if source == nil {
		return out, errors.New("operation must be an object")
	}
	if !slices.Contains([]string{"get", "post", "put", "patch", "delete"}, method) {
		return out, errors.New("HTTP method is not supported by REST tools")
	}
	if source["servers"] != nil || pathItem["servers"] != nil || source["schemes"] != nil {
		return out, errors.New("operation/path-specific servers or schemes require a separate manually configured upstream")
	}
	name := text(source["operationId"])
	if name == "" {
		name = method + "_" + path
	}
	out.Name = strings.Trim(importNameCharacters.ReplaceAllString(name, "_"), "_")
	if out.Name == "" {
		out.Name = method + "_operation"
	}
	if len(out.Name) > 112 {
		out.Name = out.Name[:112]
	}
	if out.Name != name {
		*warnings = append(*warnings, "Tool name generated or normalized; review it before adding.")
	}
	out.Description = strings.TrimSpace(text(source["summary"]) + "\n\n" + text(source["description"]))
	if utf8.RuneCountInString(out.Description) > 20000 {
		return out, errors.New("description exceeds 20000 characters")
	}
	if flag(source["deprecated"]) {
		*warnings = append(*warnings, "This operation is deprecated in the source specification.")
	}
	if source["callbacks"] != nil {
		*warnings = append(*warnings, "Callbacks are not imported.")
	}
	security, exists := source["security"]
	if !exists {
		security = i.root["security"]
	}
	if len(array(security)) > 0 {
		*warnings = append(*warnings, "This operation declares security requirements. Configure matching service credentials/headers manually; they are not imported.")
	}
	// Operation-level parameters replace inherited parameters by (in,name).
	parameters := []map[string]any{}
	positions := map[string]int{}
	for _, owner := range []map[string]any{pathItem, source} {
		if value, exists := owner["parameters"]; exists {
			if _, ok := value.([]any); !ok {
				return out, errors.New("parameters must be an array")
			}
		}
		ownerSeen := map[string]bool{}
		for _, entry := range array(owner["parameters"]) {
			p := obj(entry)
			if p == nil {
				return out, errors.New("parameter must be an object")
			}
			p, err := i.reference(p, map[string]bool{}, 0)
			if err != nil {
				return out, err
			}
			key := text(p["in"]) + ":" + text(p["name"])
			if ownerSeen[key] {
				return out, errors.New("duplicate parameters in one scope")
			}
			ownerSeen[key] = true
			if index, exists := positions[key]; exists {
				parameters[index] = p
			} else {
				positions[key] = len(parameters)
				parameters = append(parameters, p)
			}
		}
	}
	swagger := text(i.root["swagger"]) == "2.0"
	for _, p := range parameters {
		location := text(p["in"])
		if swagger && location == "body" {
			if out.BodySchema != nil {
				return out, errors.New("multiple body parameters are not supported")
			}
			consumes := i.root["consumes"]
			if v, ok := source["consumes"]; ok {
				consumes = v
			}
			if !containsJSON(consumes) {
				return out, errors.New("body must declare application/json consumes")
			}
			schema, err := i.schema(p["schema"], "input", map[string]bool{}, 0)
			if err != nil {
				return out, err
			}
			out.BodySchema = raw(schema)
			if !flag(p["required"]) {
				*warnings = append(*warnings, "The optional request body becomes a required body tool argument in this REST adapter.")
			}
			continue
		}
		if location != "path" && location != "query" {
			return out, errors.New("header, cookie and form parameters require manual configuration")
		}
		style := text(p["style"])
		if (location == "path" && style != "" && style != "simple") || (location == "query" && style != "" && style != "form") || flag(p["allowReserved"]) || flag(p["allowEmptyValue"]) || p["content"] != nil {
			return out, errors.New("parameter serialization is not supported")
		}
		var sourceSchema any = p["schema"]
		if swagger {
			m := maps.Clone(p)
			for _, key := range []string{"name", "in", "required", "collectionFormat", "description"} {
				delete(m, key)
			}
			sourceSchema = m
		}
		schema, err := i.schema(sourceSchema, "input", map[string]bool{}, 0)
		if err != nil {
			return out, err
		}
		if m := obj(schema); m != nil && m["description"] == nil && text(p["description"]) != "" {
			m["description"] = text(p["description"])
		}
		out.Parameters = append(out.Parameters, Parameter{Name: text(p["name"]), In: location, Required: location == "path" || flag(p["required"]), Schema: raw(schema)})
	}
	if !swagger && source["requestBody"] != nil {
		body, err := i.reference(obj(source["requestBody"]), map[string]bool{}, 0)
		if err != nil {
			return out, err
		}
		media := obj(obj(body["content"])["application/json"])
		if media == nil {
			return out, errors.New("request body must support application/json")
		}
		if len(obj(media["encoding"])) > 0 {
			return out, errors.New("custom request body encoding is not supported")
		}
		sourceSchema := media["schema"]
		if sourceSchema == nil {
			sourceSchema = map[string]any{}
		}
		schema, err := i.schema(sourceSchema, "input", map[string]bool{}, 0)
		if err != nil {
			return out, fmt.Errorf("request body: %w", err)
		}
		out.BodySchema = raw(schema)
		if !flag(body["required"]) {
			*warnings = append(*warnings, "The optional request body becomes a required body tool argument in this REST adapter.")
		}
	}
	schemas := []any{}
	responses := obj(source["responses"])
	if responses == nil {
		return out, errors.New("responses must declare at least one supported 2xx result")
	}
	for _, status := range slices.Sorted(maps.Keys(responses)) {
		code, err := strconv.Atoi(status)
		if err != nil || code < 200 || code > 299 {
			continue
		}
		response, err := i.reference(obj(responses[status]), map[string]bool{}, 0)
		if err != nil {
			return out, err
		}
		if response == nil {
			return out, errors.New("response must be an object")
		}
		var sourceSchema any
		if code == 204 {
			sourceSchema = map[string]any{"type": "null"}
		} else if swagger {
			produces := i.root["produces"]
			if v, ok := source["produces"]; ok {
				produces = v
			}
			if !containsJSON(produces) {
				return out, errors.New("success responses must declare application/json produces")
			}
			sourceSchema = response["schema"]
		} else {
			content := obj(response["content"])
			if media := obj(content["application/json"]); media != nil {
				sourceSchema = media["schema"]
				if sourceSchema == nil {
					sourceSchema = map[string]any{}
				}
			}
		}
		if sourceSchema == nil {
			return out, fmt.Errorf("HTTP %d requires a JSON response schema/content; only 204 supports an empty body", code)
		}
		schema, err := i.schema(sourceSchema, "output", map[string]bool{}, 0)
		if err != nil {
			return out, fmt.Errorf("response HTTP %d: %w", code, err)
		}
		schemas = append(schemas, schema)
		out.SuccessStatuses = append(out.SuccessStatuses, code)
	}
	if len(schemas) == 0 {
		return out, errors.New("declare explicit supported 2xx responses; default/range responses alone are not imported")
	}
	if len(schemas) == 1 {
		out.ResponseSchema = raw(schemas[0])
	} else {
		out.ResponseSchema = raw(map[string]any{"anyOf": schemas})
	}
	if responses["2XX"] != nil || responses["default"] != nil {
		*warnings = append(*warnings, "Only explicit 2xx success statuses are imported; default/range responses are excluded.")
	}
	c := Config{Auth: Auth{Type: "none"}, TimeoutSeconds: 30, Operations: []Operation{out}}
	if _, err := Compile("https://import.invalid", c, nil); err != nil {
		return out, fmt.Errorf("REST compatibility: %w", err)
	}
	return out, nil
}
func containsJSON(value any) bool {
	for _, v := range array(value) {
		if text(v) == "application/json" {
			return true
		}
	}
	return false
}
