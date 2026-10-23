package store

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/bishoku/vmcp/core/internal/audit"
	"github.com/bishoku/vmcp/core/internal/mcp"
)

type CatalogDelta struct {
	Added   []string `json:"added,omitempty"`
	Changed []string `json:"changed,omitempty"`
	Removed []string `json:"removed,omitempty"`
}

func (d CatalogDelta) empty() bool { return len(d.Added)+len(d.Changed)+len(d.Removed) == 0 }

type DiscoveryChanges struct {
	Tools        CatalogDelta `json:"tools"`
	Resources    CatalogDelta `json:"resources"`
	Templates    CatalogDelta `json:"templates"`
	Prompts      CatalogDelta `json:"prompts"`
	Capabilities bool         `json:"capabilitiesChanged"`
	Instructions bool         `json:"instructionsChanged"`
}

func (d DiscoveryChanges) Empty() bool {
	return d.Tools.empty() && d.Resources.empty() && d.Templates.empty() && d.Prompts.empty() && !d.Capabilities && !d.Instructions
}

// Reason contains identifiers only, never discovered schemas, descriptions or instructions.
// Large changes retain exact counts instead of an unbounded metadata document.
func (d DiscoveryChanges) Reason() string {
	raw, _ := json.Marshal(d)
	if len(raw) <= 8192 {
		return string(raw)
	}
	counts := func(x CatalogDelta) string {
		return fmt.Sprintf("added=%d changed=%d removed=%d", len(x.Added), len(x.Changed), len(x.Removed))
	}
	return fmt.Sprintf("tools %s; resources %s; templates %s; prompts %s; capabilities_changed=%t; instructions_changed=%t; identifiers omitted (summary limit)", counts(d.Tools), counts(d.Resources), counts(d.Templates), counts(d.Prompts), d.Capabilities, d.Instructions)
}
func catalogDelta(old, next []mcp.CatalogObject, key string) CatalogDelta {
	before, after := map[string][]byte{}, map[string][]byte{}
	for _, o := range old {
		before[o.String(key)], _ = json.Marshal(o)
	}
	for _, o := range next {
		after[o.String(key)], _ = json.Marshal(o)
	}
	var delta CatalogDelta
	for name, value := range after {
		if previous, ok := before[name]; !ok {
			delta.Added = append(delta.Added, name)
		} else if !jsonEqual(previous, value) {
			delta.Changed = append(delta.Changed, name)
		}
	}
	for name := range before {
		if _, ok := after[name]; !ok {
			delta.Removed = append(delta.Removed, name)
		}
	}
	slices.Sort(delta.Added)
	slices.Sort(delta.Changed)
	slices.Sort(delta.Removed)
	return delta
}

// ReplaceDiscovery is called under Store.Do's revision lock. No-op discovery
// updates freshness only; meaningful changes stage one audited mutation.
func (t *Tx) ReplaceDiscovery(ctx context.Context, slug string, tools []mcp.Tool, catalog mcp.Catalog, at time.Time, event audit.Event) (DiscoveryChanges, error) {
	old, err := t.ListCatalog(ctx, slug)
	if err != nil {
		return DiscoveryChanges{}, err
	}
	before, _ := json.Marshal(old.Capabilities)
	after, _ := json.Marshal(catalog.Capabilities)
	changes := DiscoveryChanges{Resources: catalogDelta(old.Resources, catalog.Resources, "uri"), Templates: catalogDelta(old.Templates, catalog.Templates, "uriTemplate"), Prompts: catalogDelta(old.Prompts, catalog.Prompts, "name"), Capabilities: !jsonEqual(before, after), Instructions: old.Instructions != catalog.Instructions}
	result, err := t.ReplaceTools(ctx, slug, tools, at)
	if err != nil {
		return changes, err
	}
	changes.Tools = CatalogDelta{Added: result.Added, Changed: result.Changed, Removed: result.Removed}
	if !changes.Resources.empty() || !changes.Templates.empty() || !changes.Prompts.empty() || changes.Capabilities || changes.Instructions {
		if err = t.ReplaceCatalog(ctx, slug, catalog); err != nil {
			return changes, err
		}
	}
	if err = t.SetRefreshStatus(ctx, slug, at, nil); err != nil {
		return changes, err
	}
	if !changes.Empty() {
		event.Reason = changes.Reason()
		event.Target = "upstream:" + slug
		t.mutation = &event
	}
	return changes, nil
}
