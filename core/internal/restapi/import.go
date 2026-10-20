package restapi

import (
	"encoding/json"
	"errors"
	"io"
	"maps"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/bishoku/vmcp/core/internal/netpolicy"
	"go.yaml.in/yaml/v3"
)

const MaxImportBytes = 2 << 20
const maxImportOperations = 1000

// ImportResult is a preview only. Credentials and server settings are never imported.
type ImportResult struct {
	Title      string            `json:"title"`
	Version    string            `json:"version"`
	Servers    []string          `json:"servers"`
	Warnings   []string          `json:"warnings"`
	Operations []ImportOperation `json:"operations"`
}
type ImportOperation struct {
	ID        string     `json:"id"`
	Method    string     `json:"method"`
	Path      string     `json:"path"`
	Summary   string     `json:"summary"`
	Tags      []string   `json:"tags"`
	Operation *Operation `json:"operation,omitempty"`
	Warnings  []string   `json:"warnings"`
	Errors    []string   `json:"errors"`
}

type importer struct {
	root   map[string]any
	legacy bool
	nodes  int
	bytes  int
}

func obj(v any) map[string]any  { m, _ := v.(map[string]any); return m }
func text(v any) string         { s, _ := v.(string); return s }
func flag(v any) bool           { b, _ := v.(bool); return b }
func array(v any) []any         { a, _ := v.([]any); return a }
func raw(v any) json.RawMessage { b, _ := json.Marshal(v); return b }

// Import accepts a single JSON/YAML document, with no filesystem/network resolution.
// Conversion failures belong to individual operations so unrelated operations remain selectable.
func Import(document string) (*ImportResult, error) {
	if len(document) == 0 || len(document) > MaxImportBytes || !utf8.ValidString(document) {
		return nil, errors.New("provide a UTF-8 JSON or YAML document up to 2 MiB")
	}
	decoder := yaml.NewDecoder(strings.NewReader(document))
	var node yaml.Node
	if decoder.Decode(&node) != nil {
		return nil, errors.New("invalid JSON or YAML document")
	}
	var extra yaml.Node
	if decoder.Decode(&extra) != io.EOF {
		return nil, errors.New("provide exactly one document")
	}
	count := 0
	value, err := yamlValue(&node, 0, &count)
	if err != nil {
		return nil, err
	}
	root := obj(value)
	version := text(root["openapi"])
	legacy := text(root["swagger"]) == "2.0"
	if legacy {
		version = "2.0"
	} else if !strings.HasPrefix(version, "3.0.") && !strings.HasPrefix(version, "3.1.") {
		return nil, errors.New("supported versions: Swagger 2.0 and OpenAPI 3.0/3.1")
	}
	if root["swagger"] != nil && root["openapi"] != nil {
		return nil, errors.New("ambiguous specification version")
	}
	if dialect, exists := root["jsonSchemaDialect"]; exists && text(dialect) != "https://json-schema.org/draft/2020-12/schema" && text(dialect) != "https://spec.openapis.org/oas/3.1/dialect/base" {
		return nil, errors.New("unsupported document schema dialect")
	}
	paths := obj(root["paths"])
	if paths == nil {
		return nil, errors.New("paths must be an object")
	}
	result := &ImportResult{Title: text(obj(root["info"])["title"]), Version: version, Servers: []string{}, Warnings: []string{"Base URL, authentication, headers, TLS and identity forwarding remain unchanged. Review them before saving.", "Only JSON operations are converted. No operations are selected automatically; GET operations are not automatically marked read-only."}, Operations: []ImportOperation{}}
	if legacy {
		host := text(root["host"])
		base := text(root["basePath"])
		if len(array(root["schemes"])) == 0 {
			if host != "" {
				result.Servers = append(result.Servers, "//"+host+base)
			} else if base != "" {
				result.Servers = append(result.Servers, base)
			}
		}
		for _, scheme := range array(root["schemes"]) {
			candidate := text(scheme) + "://" + host + base
			if host != "" && netpolicy.ValidateURL(candidate) == nil {
				result.Servers = append(result.Servers, candidate)
			}
		}
	} else {
		for _, s := range array(root["servers"]) {
			candidate := text(obj(s)["url"])
			if candidate != "" {
				result.Servers = append(result.Servers, candidate)
			}
		}
	}
	names := map[string]int{}
	imp := importer{root: root, legacy: legacy || strings.HasPrefix(version, "3.0.")}
	for _, path := range slices.Sorted(maps.Keys(paths)) {
		if len(result.Operations) >= maxImportOperations {
			return nil, errors.New("at most 1000 source operations may be imported")
		}
		if strings.HasPrefix(path, "x-") {
			continue
		}
		pathItem := obj(paths[path])
		if pathItem == nil {
			return nil, errors.New("path items must be objects")
		}
		resolved, resolveErr := imp.reference(pathItem, map[string]bool{}, 0)
		if resolveErr == nil {
			pathItem = resolved
		}
		methods := []string{"get", "post", "put", "patch", "delete", "head", "options", "trace"}
		// A broken Path Item reference is visible, never silently ignored.
		if resolveErr != nil {
			result.Operations = append(result.Operations, ImportOperation{ID: "PATH " + path, Path: path, Tags: []string{}, Warnings: []string{}, Errors: []string{resolveErr.Error()}})
			continue
		}
		for _, method := range methods {
			value, exists := pathItem[method]
			if !exists {
				continue
			}
			if len(result.Operations) >= maxImportOperations {
				return nil, errors.New("at most 1000 source operations may be imported")
			}
			source := obj(value)
			candidate := ImportOperation{ID: strings.ToUpper(method) + " " + path, Method: strings.ToUpper(method), Path: path, Summary: text(source["summary"]), Tags: []string{}, Warnings: []string{}, Errors: []string{}}
			for _, tag := range array(source["tags"]) {
				if s := text(tag); s != "" {
					candidate.Tags = append(candidate.Tags, s)
				}
			}
			operation, err := imp.operation(path, method, pathItem, source, &candidate.Warnings)
			if err != nil {
				candidate.Errors = append(candidate.Errors, err.Error())
			} else {
				base := operation.Name
				for suffix := 2; names[operation.Name] > 0; suffix++ {
					operation.Name = base + "_" + strconv.Itoa(suffix)
				}
				names[operation.Name] = 1
				if operation.Name != base {
					candidate.Warnings = append(candidate.Warnings, "Duplicate tool name renamed; review before adding.")
				}

				candidate.Operation = &operation
			}
			result.Operations = append(result.Operations, candidate)
		}
	}
	if len(result.Operations) == 0 {
		return nil, errors.New("no operations found in paths")
	}
	encoded, _ := json.Marshal(result)
	if len(encoded) > 8<<20 {
		return nil, errors.New("expanded import exceeds 8 MiB; split the specification")
	}
	return result, nil
}

func yamlValue(n *yaml.Node, depth int, count *int) (any, error) {
	*count++
	if depth > 64 || *count > 200000 {
		return nil, errors.New("document nesting or node limit exceeded")
	}
	switch n.Kind {
	case yaml.DocumentNode:
		if len(n.Content) != 1 {
			return nil, errors.New("invalid document")
		}
		return yamlValue(n.Content[0], depth+1, count)
	case yaml.AliasNode:
		return nil, errors.New("YAML aliases are not supported; use local OpenAPI references")
	case yaml.MappingNode:
		if n.Tag != "!!map" {
			return nil, errors.New("custom YAML tags are not supported")
		}
		out := map[string]any{}
		for i := 0; i < len(n.Content); i += 2 {
			key := n.Content[i]
			if key.Kind != yaml.ScalarNode || (key.Tag != "!!str" && key.Tag != "!!int") {
				return nil, errors.New("object keys must be strings or HTTP status codes")
			}
			if _, exists := out[key.Value]; exists {
				return nil, errors.New("duplicate object keys are not allowed")
			}
			v, err := yamlValue(n.Content[i+1], depth+1, count)
			if err != nil {
				return nil, err
			}
			out[key.Value] = v
		}
		return out, nil
	case yaml.SequenceNode:
		if n.Tag != "!!seq" {
			return nil, errors.New("custom YAML tags are not supported")
		}
		out := []any{}
		for _, child := range n.Content {
			v, err := yamlValue(child, depth+1, count)
			if err != nil {
				return nil, err
			}
			out = append(out, v)
		}
		return out, nil
	case yaml.ScalarNode:
		switch n.Tag {
		case "!!str", "!!timestamp":
			return n.Value, nil
		case "!!null":
			return nil, nil
		case "!!bool":
			return strings.EqualFold(n.Value, "true"), nil
		case "!!int", "!!float":
			// Preserve JSON numbers exactly; YAML-specific numeric syntax is normalized.
			var number any
			if decodeJSON([]byte(n.Value), &number) == nil {
				if _, ok := number.(json.Number); ok {
					return number, nil
				}
			}
			var v any
			if n.Decode(&v) == nil {
				b, err := json.Marshal(v)
				if err == nil {
					return json.Number(b), nil
				}
			}
		}
	}
	return nil, errors.New("unsupported YAML tag or value")
}

// reference resolves only document-local JSON Pointers. Siblings are conservatively
// rejected rather than accidentally weakening a schema or merging conflicting Path Items.
func (i *importer) reference(source map[string]any, seen map[string]bool, depth int) (map[string]any, error) {
	if depth > 32 {
		return nil, errors.New("reference depth limit exceeded")
	}
	refValue, exists := source["$ref"]
	if !exists {
		return source, nil
	}
	ref, ok := refValue.(string)
	if !ok || !strings.HasPrefix(ref, "#/") {
		return nil, errors.New("only local #/ references are supported; external references are never fetched")
	}
	if seen[ref] {
		return nil, errors.New("recursive references cannot be converted to inline REST schemas")
	}
	for k := range source {
		if k != "$ref" && k != "description" && k != "summary" {
			return nil, errors.New("reference siblings require manual schema review")
		}
	}
	decoded, err := url.PathUnescape(ref[2:])
	if err != nil {
		return nil, errors.New("invalid local reference")
	}
	var current any = i.root
	for _, part := range strings.Split(decoded, "/") {
		part = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
		switch v := current.(type) {
		case map[string]any:
			current = v[part]
		case []any:
			n, err := strconv.Atoi(part)
			if err != nil || n < 0 || n >= len(v) {
				return nil, errors.New("local reference not found")
			}
			current = v[n]
		default:
			return nil, errors.New("local reference not found")
		}
	}
	target := obj(current)
	if target == nil {
		return nil, errors.New("local reference must resolve to an object")
	}
	next := maps.Clone(seen)
	next[ref] = true
	resolved, err := i.reference(target, next, depth+1)
	if err != nil {
		return nil, err
	}
	resolved = maps.Clone(resolved)
	for _, key := range []string{"description", "summary"} {
		if v, ok := source[key]; ok {
			resolved[key] = v
		}
	}
	return resolved, nil
}
