package upstream

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/bishoku/vmcp/core/internal/mcp"
	"github.com/yosida95/uritemplate/v3"
	"net/url"
)

// DiscoverCatalog only requests advertised feature families. A malformed page
// fails the refresh atomically rather than replacing good discovery with guesses.
func DiscoverCatalog(ctx context.Context, c Conn, caps mcp.ServerCapabilities) (mcp.Catalog, error) {
	out := mcp.Catalog{Capabilities: caps, Resources: []mcp.CatalogObject{}, Templates: []mcp.CatalogObject{}, Prompts: []mcp.CatalogObject{}}
	families := []struct {
		method, field, key string
		enabled            bool
		target             *[]mcp.CatalogObject
	}{
		{"resources/list", "resources", "uri", caps.Resources != nil, &out.Resources},
		{"resources/templates/list", "resourceTemplates", "uriTemplate", caps.Resources != nil, &out.Templates},
		{"prompts/list", "prompts", "name", caps.Prompts != nil, &out.Prompts},
	}
	for _, f := range families {
		if !f.enabled {
			continue
		}
		seen, cursor, keys, total := map[string]bool{}, "", map[string]bool{}, 0
		for page := 0; ; page++ {
			if page >= maxDiscoveryPages {
				return out, errors.New("catalog page limit exceeded")
			}
			raw, err := Call(ctx, c, f.method, map[string]string{"cursor": cursor})
			if err != nil {
				return out, err
			}
			total += len(raw)
			if total > 10<<20 {
				return out, errors.New("catalog byte limit exceeded")
			}
			var fields map[string]json.RawMessage
			if json.Unmarshal(raw, &fields) != nil || fields[f.field] == nil {
				return out, errors.New("invalid catalog page")
			}
			var objects []mcp.CatalogObject
			if json.Unmarshal(fields[f.field], &objects) != nil || objects == nil {
				return out, errors.New("invalid catalog objects")
			}
			if len(*f.target)+len(objects) > maxDiscoveryTools {
				return out, errors.New("catalog object limit exceeded")
			}
			for _, o := range objects {
				key := o.String(f.key)
				if key == "" || len(key) > 4096 || o.String("name") == "" || keys[key] {
					return out, errors.New("invalid or duplicate catalog identity")
				}
				if f.key == "uri" {
					u, err := url.Parse(key)
					if err != nil || u.Scheme == "" {
						return out, errors.New("resource URI must be absolute")
					}
				}
				if f.key == "uriTemplate" {
					tpl, err := uritemplate.New(key)
					if err != nil || len(tpl.Varnames()) > 32 {
						return out, errors.New("invalid or oversized resource template")
					}
				}
				if f.key == "name" && o["arguments"] != nil {
					var arguments []struct {
						Name     string `json:"name"`
						Required bool   `json:"required"`
					}
					if json.Unmarshal(o["arguments"], &arguments) != nil || len(arguments) > 128 {
						return out, errors.New("invalid prompt arguments")
					}
					names := map[string]bool{}
					for _, arg := range arguments {
						if arg.Name == "" || len(arg.Name) > 128 || names[arg.Name] {
							return out, errors.New("invalid prompt argument identity")
						}
						names[arg.Name] = true
					}
				}
				keys[key] = true
				*f.target = append(*f.target, o)
			}
			var next string
			if fields["nextCursor"] != nil && json.Unmarshal(fields["nextCursor"], &next) != nil {
				return out, errors.New("invalid catalog cursor")
			}
			if next == "" {
				break
			}
			if len(next) > 4096 || seen[next] {
				return out, errors.New("repeated catalog cursor")
			}
			seen[next] = true
			cursor = next
		}
	}
	return out, nil
}

func (c *SSEClient) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	return c.call(ctx, method, params)
}

func Call(ctx context.Context, c Conn, method string, params any) (json.RawMessage, error) {
	caller, ok := c.(interface {
		Call(context.Context, string, any) (json.RawMessage, error)
	})
	if !ok {
		return nil, errors.New("upstream does not support generic requests")
	}
	return caller.Call(ctx, method, params)
}
