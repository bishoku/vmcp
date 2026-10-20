package config

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/bishoku/vmcp/core/internal/mcp"
)

// Guidance is authored configuration, independent of upstream catalog selections.
type Guidance struct {
	Instructions         string                  `json:"instructions"`
	Resources            []LocalResource         `json:"resources"`
	Prompts              []LocalPrompt           `json:"prompts"`
	UpstreamInstructions []string                `json:"upstreamInstructions"`
	ToolReferences       []GuidanceToolReference `json:"toolReferences"`
}
type GuidanceToolReference struct {
	Key      string `json:"key"`
	Upstream string `json:"upstream"`
	Tool     string `json:"tool"`
}
type LocalResource struct {
	Name        string `json:"name"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
	MimeType    string `json:"mimeType"`
	Text        string `json:"text"`
}
type PromptArgument struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}
type PromptMessage struct {
	Role string `json:"role"`
	Text string `json:"text"`
}
type LocalPrompt struct {
	Name        string           `json:"name"`
	Title       string           `json:"title"`
	Description string           `json:"description"`
	Enabled     bool             `json:"enabled"`
	Arguments   []PromptArgument `json:"arguments"`
	Messages    []PromptMessage  `json:"messages"`
}
type LocalContent struct {
	ToolReferences map[string]string
	Text           string
	MimeType       string
	Messages       []PromptMessage
}

var GuidanceToken = regexp.MustCompile(`\{\{(tool|arg):([a-zA-Z0-9_.-]+)\}\}`)

func LocalResourceURI(name string) string        { return "vmcp://guidance/resource/" + name }
func LocalPromptName(name string) string         { return "vmcp_local__" + name }
func UpstreamInstructionsURI(slug string) string { return "vmcp://guidance/upstream/" + slug }
func (g Guidance) Empty() bool {
	return g.Instructions == "" && len(g.Resources) == 0 && len(g.Prompts) == 0 && len(g.UpstreamInstructions) == 0 && len(g.ToolReferences) == 0
}

// RenderGuidance is a single substitution pass: argument values cannot introduce templates.
func RenderGuidance(text string, tools, args map[string]string, keepArgs bool) (string, error) {
	var out strings.Builder
	count, cursor := 0, 0
	appendText := func(value string) bool {
		count += utf8.RuneCountInString(value)
		if count > 100000 {
			return false
		}
		out.WriteString(value)
		return true
	}
	for _, match := range GuidanceToken.FindAllStringSubmatchIndex(text, -1) {
		if !appendText(text[cursor:match[0]]) {
			return "", fmt.Errorf("rendered guidance too large")
		}
		kind, key := text[match[2]:match[3]], text[match[4]:match[5]]
		value := text[match[0]:match[1]]
		if kind != "arg" || !keepArgs {
			values := tools
			if kind == "arg" {
				values = args
			}
			var ok bool
			value, ok = values[key]
			if !ok {
				return "", fmt.Errorf("unavailable guidance reference")
			}
		}
		// Bound expansion before writing: repeated large argument values must not
		// allocate an unbounded intermediate string under concurrent calls.
		if !appendText(value) {
			return "", fmt.Errorf("rendered guidance too large")
		}
		cursor = match[1]
	}
	if !appendText(text[cursor:]) {
		return "", fmt.Errorf("rendered guidance too large")
	}
	return out.String(), nil
}

// WithGuidance appends local definitions to the existing catalog before indexing it.
// References to tools no longer exposed by policy/discovery fail closed per item.
func (v VirtualServer) WithGuidance(upstreams map[string]Upstream) VirtualServer {
	if v.Guidance.Empty() {
		return v
	}
	v.Guidance = v.Guidance.Normalized()
	refs := map[string]string{}
	if len(v.Guidance.ToolReferences) > 0 {
		exposed := make(map[string]string, len(v.Tools))
		for _, tool := range v.Tools {
			exposed[upstreams[tool.UpstreamID].Slug+"\x00"+tool.UpstreamName] = tool.Tool.Name
		}
		for _, ref := range v.Guidance.ToolReferences {
			if name, ok := exposed[ref.Upstream+"\x00"+ref.Tool]; ok {
				refs[ref.Key] = name
			}
		}
	}
	v.Instructions, _ = RenderGuidance(v.Guidance.Instructions, refs, nil, false)
	if v.PII != nil && v.Instructions != "" {
		raw, _ := json.Marshal(v.Instructions)
		masked, _, err := v.PII.MaskJSON(raw)
		if err != nil {
			v.Instructions = ""
		} else {
			_ = json.Unmarshal(masked, &v.Instructions)
		}
	}
	addResource := func(uri, name, title, description, mime, text string) {
		v.Catalog = append(v.Catalog, VirtualCatalogObject{Kind: "resource", Original: uri, Exposed: uri, Definition: mcp.CatalogObject{}.With("uri", uri).With("name", name).With("title", title).With("description", description).With("mimeType", mime), Local: &LocalContent{Text: text, MimeType: mime}})
	}
	for _, r := range v.Guidance.Resources {
		if !r.Enabled {
			continue
		}
		text, err := RenderGuidance(r.Text, refs, nil, false)
		if err != nil {
			continue
		}
		addResource(LocalResourceURI(r.Name), r.Name, r.Title, r.Description, r.MimeType, text)
	}
	for _, p := range v.Guidance.Prompts {
		if !p.Enabled {
			continue
		}
		messages := make([]PromptMessage, 0, len(p.Messages))
		valid := true
		for _, m := range p.Messages {
			_, err := RenderGuidance(m.Text, refs, nil, true)
			if err != nil {
				valid = false
				break
			}
			messages = append(messages, PromptMessage{Role: m.Role, Text: m.Text})
		}
		if !valid {
			continue
		}
		name := LocalPromptName(p.Name)
		definition := mcp.CatalogObject{}.With("name", name).With("title", p.Title).With("description", p.Description)
		definition["arguments"], _ = json.Marshal(p.Arguments)
		v.Catalog = append(v.Catalog, VirtualCatalogObject{Kind: "prompt", Original: name, Exposed: name, Definition: definition, Local: &LocalContent{Messages: messages, ToolReferences: refs}})
	}
	if len(v.Guidance.UpstreamInstructions) == 0 {
		return v
	}
	bySlug := map[string]Upstream{}
	for _, u := range upstreams {
		bySlug[u.Slug] = u
	}
	for _, slug := range v.Guidance.UpstreamInstructions {
		u, ok := bySlug[slug]
		if !ok || strings.TrimSpace(u.Instructions) == "" {
			continue
		}
		addResource(UpstreamInstructionsURI(slug), slug+"-instructions", slug+" instructions", "Current instructions from upstream "+slug, "text/plain", u.Instructions)
	}
	return v
}

// GuidancePatch replaces only explicitly supplied members; arrays replace as a whole.
type GuidancePatch struct {
	Instructions         *string                  `json:"instructions"`
	Resources            *[]LocalResource         `json:"resources"`
	Prompts              *[]LocalPrompt           `json:"prompts"`
	UpstreamInstructions *[]string                `json:"upstreamInstructions"`
	ToolReferences       *[]GuidanceToolReference `json:"toolReferences"`
}

func (p GuidancePatch) Apply(g Guidance) Guidance {
	if p.Instructions != nil {
		g.Instructions = *p.Instructions
	}
	if p.Resources != nil {
		g.Resources = *p.Resources
	}
	if p.Prompts != nil {
		g.Prompts = *p.Prompts
	}
	if p.UpstreamInstructions != nil {
		g.UpstreamInstructions = *p.UpstreamInstructions
	}
	if p.ToolReferences != nil {
		g.ToolReferences = *p.ToolReferences
	}
	return g
}

// Normalized makes empty collections stable across UI, API and export round trips.
func (g Guidance) Normalized() Guidance {
	if g.Resources == nil {
		g.Resources = []LocalResource{}
	}
	g.Prompts = append([]LocalPrompt{}, g.Prompts...)
	for i := range g.Prompts {
		if g.Prompts[i].Arguments == nil {
			g.Prompts[i].Arguments = []PromptArgument{}
		}
		if g.Prompts[i].Messages == nil {
			g.Prompts[i].Messages = []PromptMessage{}
		}
	}
	if g.UpstreamInstructions == nil {
		g.UpstreamInstructions = []string{}
	}
	if g.ToolReferences == nil {
		g.ToolReferences = []GuidanceToolReference{}
	}
	return g
}
