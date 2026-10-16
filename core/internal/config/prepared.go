package config

import (
	"encoding/json"
	"errors"
	"regexp"
	"strings"

	"github.com/bishoku/vmcp/core/internal/mcp"
	"github.com/yosida95/uritemplate/v3"
)

type catalogKey struct{ kind, name string }
type compiledTemplate struct {
	index  int
	prefix string
	re     *regexp.Regexp
}
type preparedCatalog struct {
	capabilities mcp.ServerCapabilities
	index        map[catalogKey]int
	templates    []compiledTemplate
	lists        map[string][]mcp.CatalogObject
	errors       map[string]error
	tools        []mcp.Tool
	toolErrors   []error
}

// WithPreparedMetadata prepares only immutable snapshot data. Failures are retained
// for the same list request/page that previously failed, not promoted to load errors.
func (v VirtualServer) WithPreparedMetadata() VirtualServer {
	v.prepared = nil
	p := &preparedCatalog{index: map[catalogKey]int{}, lists: map[string][]mcp.CatalogObject{}, errors: map[string]error{}}
	for i, o := range v.Catalog {
		key := catalogKey{o.Kind, o.Exposed}
		if _, exists := p.index[key]; !exists {
			p.index[key] = i
		}
		if o.Kind == "template" {
			if tpl, err := uritemplate.New(o.Original); err == nil {
				p.templates = append(p.templates, compiledTemplate{i, strings.TrimSuffix(o.Exposed, o.Original), tpl.Regexp()})
			}
		}
		safe, err := v.safeCatalog(o)
		if err != nil && p.errors[o.Kind] == nil {
			p.errors[o.Kind] = err
		}
		p.lists[o.Kind] = append(p.lists[o.Kind], safe)
	}
	p.tools = make([]mcp.Tool, len(v.Tools))
	p.toolErrors = make([]error, len(v.Tools))
	for i, t := range v.Tools {
		p.tools[i], p.toolErrors[i] = v.safeTool(t.Tool)
	}
	p.capabilities = v.CatalogCapabilities()
	v.prepared = p
	return v
}

// CatalogRoute retains first-match and template precedence in operator-defined order.
func (v VirtualServer) CatalogRoute(kind, exposed string, expand bool) (VirtualCatalogObject, string, bool) {
	if len(exposed) > 8192 {
		return VirtualCatalogObject{}, "", false
	}
	if v.prepared != nil {
		if i, ok := v.prepared.index[catalogKey{kind, exposed}]; ok {
			o := v.Catalog[i]
			return o, o.Original, true
		}
		if expand && kind == "resource" {
			for _, tpl := range v.prepared.templates {
				if strings.HasPrefix(exposed, tpl.prefix) {
					original := strings.TrimPrefix(exposed, tpl.prefix)
					if tpl.re.MatchString(original) {
						return v.Catalog[tpl.index], original, true
					}
				}
			}
		}
		return VirtualCatalogObject{}, "", false
	}
	for _, o := range v.Catalog {
		if o.Kind == kind && o.Exposed == exposed {
			return o, o.Original, true
		}
	}
	if expand && kind == "resource" {
		for _, o := range v.Catalog {
			if o.Kind != "template" {
				continue
			}
			prefix := strings.TrimSuffix(o.Exposed, o.Original)
			if !strings.HasPrefix(exposed, prefix) {
				continue
			}
			original := strings.TrimPrefix(exposed, prefix)
			tpl, err := uritemplate.New(o.Original)
			if err == nil && tpl.Regexp().MatchString(original) {
				return o, original, true
			}
		}
	}
	return VirtualCatalogObject{}, "", false
}
func (v VirtualServer) safeTool(tool mcp.Tool) (mcp.Tool, error) {
	if v.PII == nil {
		return tool, nil
	}
	raw, err := json.Marshal(tool)
	if err != nil {
		return mcp.Tool{}, err
	}
	masked, _, err := v.PII.MaskJSON(raw)
	if err != nil {
		return mcp.Tool{}, err
	}
	var safe mcp.Tool
	if err = json.Unmarshal(masked, &safe); err != nil {
		return mcp.Tool{}, err
	}
	safe.Name = tool.Name
	return safe, nil
}
func (v VirtualServer) safeCatalog(o VirtualCatalogObject) (mcp.CatalogObject, error) {
	if v.PII == nil {
		return o.Definition, nil
	}
	raw, err := json.Marshal(o.Definition)
	if err != nil {
		return nil, err
	}
	masked, _, err := v.PII.MaskJSON(raw)
	if err != nil {
		return nil, err
	}
	var safe mcp.CatalogObject
	if err = json.Unmarshal(masked, &safe); err != nil {
		return nil, err
	}
	key := "uri"
	if o.Kind == "template" {
		key = "uriTemplate"
	}
	if o.Kind == "prompt" {
		key = "name"
	}
	if safe == nil {
		return nil, errors.New("catalog metadata must be an object")
	}
	safe[key] = o.Definition[key]
	return safe, nil
}

// ListedCatalog is read-only, like the snapshot itself. Metadata errors remain
// whole-kind errors before pagination, matching the original handler behavior.
func (v VirtualServer) ListedCatalog(kind string) ([]mcp.CatalogObject, error) {
	if v.prepared != nil {
		list := v.prepared.lists[kind]
		if list == nil {
			list = []mcp.CatalogObject{}
		}
		return list, v.prepared.errors[kind]
	}
	out := []mcp.CatalogObject{}
	for _, o := range v.Catalog {
		if o.Kind == kind {
			safe, err := v.safeCatalog(o)
			if err != nil {
				return nil, err
			}
			out = append(out, safe)
		}
	}
	return out, nil
}
func (v VirtualServer) ListedTools(start, end int) ([]mcp.Tool, error) {
	out := make([]mcp.Tool, 0, end-start)
	for i := start; i < end; i++ {
		var tool mcp.Tool
		var err error
		if v.prepared != nil {
			tool, err = v.prepared.tools[i], v.prepared.toolErrors[i]
		} else {
			tool, err = v.safeTool(v.Tools[i].Tool)
		}
		if err != nil {
			return nil, err
		}
		out = append(out, tool)
	}
	return out, nil
}
