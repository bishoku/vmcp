package config

import (
	"encoding/base64"
	"encoding/json"
	"github.com/bishoku/vmcp/core/internal/mcp"
)

type VirtualCatalogObject struct {
	UpstreamID string
	Kind       string
	Original   string
	Exposed    string
	Definition mcp.CatalogObject
	Subscribe  bool
	Complete   bool
}

func CatalogName(upstream, kind, name, alias string) string {
	if kind == "prompt" {
		return ExposedName(upstream, name, alias)
	}
	if kind == "template" {
		return "vmcp://" + upstream + "/template/" + name
	}
	if alias != "" {
		return "vmcp://" + upstream + "/alias/" + alias
	}
	return "vmcp://" + upstream + "/resource/" + base64.RawURLEncoding.EncodeToString([]byte(name))
}
func (v VirtualServer) CatalogList(kind string) []mcp.CatalogObject {
	out := []mcp.CatalogObject{}
	for _, o := range v.Catalog {
		if o.Kind == kind {
			out = append(out, o.Definition)
		}
	}
	return out
}
func (v VirtualServer) CatalogCapabilities() mcp.ServerCapabilities {
	if v.prepared != nil {
		caps := v.prepared.capabilities
		if caps.Resources != nil {
			copy := *caps.Resources
			caps.Resources = &copy
		}
		if caps.Prompts != nil {
			copy := *caps.Prompts
			caps.Prompts = &copy
		}
		return caps
	}
	caps := mcp.ServerCapabilities{}
	for _, o := range v.Catalog {
		switch o.Kind {
		case "resource", "template":
			if caps.Resources == nil {
				caps.Resources = &mcp.ResourcesCapability{}
			}
			caps.Resources.Subscribe = caps.Resources.Subscribe || o.Subscribe
		case "prompt":
			caps.Prompts = &mcp.ToolsCapability{}
		}
	}
	for _, o := range v.Catalog {
		if o.Complete {
			caps.Completions = &struct{}{}
		}
	}
	return caps
}
func CatalogDefinition(raw []byte, kind, exposed, description string) (mcp.CatalogObject, error) {
	var o mcp.CatalogObject
	if err := json.Unmarshal(raw, &o); err != nil {
		return nil, err
	}
	key := "uri"
	if kind == "template" {
		key = "uriTemplate"
	}
	if kind == "prompt" {
		key = "name"
	}
	o = o.With(key, exposed)
	if description != "" {
		o = o.With("description", description)
	}
	return o, nil
}
