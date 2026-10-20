package store

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"unicode/utf8"

	"github.com/bishoku/vmcp/core/internal/apierror"
	"github.com/bishoku/vmcp/core/internal/config"
)

var guidanceKey = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.-]{0,63}$`)

func validateGuidance(in VirtualServerInput) []apierror.FieldError {
	if in.Guidance == nil {
		return nil
	}
	g := in.Guidance
	var errs []apierror.FieldError
	check := func(ok bool, field, message string) {
		if !ok {
			errs = append(errs, vf("/guidance/"+field, message))
		}
	}
	size := func(text string, max int, field string) {
		check(utf8.RuneCountInString(text) <= max, field, fmt.Sprintf("must be at most %d characters", max))
	}
	size(g.Instructions, 4000, "instructions")
	raw, _ := json.Marshal(g)
	check(len(raw) <= 512<<10, "", "guidance must fit in 512 KiB")
	check(len(g.Resources) <= 100, "resources", "at most 100 local resources")
	check(len(g.Prompts) <= 100, "prompts", "at most 100 local prompts")
	check(len(g.UpstreamInstructions) <= 100, "upstreamInstructions", "at most 100 upstream instruction selections")
	check(len(g.ToolReferences) <= 100, "toolReferences", "at most 100 tool references")
	tools := map[string]bool{}
	for _, t := range in.Tools {
		tools[t.Upstream+"\x00"+t.Tool] = true
	}
	refs := map[string]bool{}
	for i, r := range g.ToolReferences {
		p := fmt.Sprintf("toolReferences/%d", i)
		check(guidanceKey.MatchString(r.Key) && !refs[r.Key], p+"/key", "must be a unique reference key (1–64 letters, digits, dot, underscore or hyphen; start with a letter)")
		refs[r.Key] = true
		check(tools[r.Upstream+"\x00"+r.Tool], p+"/tool", "reference must select a tool included in this virtual server")
	}
	tokens := func(text, field string, args map[string]bool) {
		for _, m := range config.GuidanceToken.FindAllStringSubmatch(text, -1) {
			if m[1] == "tool" {
				check(refs[m[2]], field, "unknown tool reference: "+m[2])
			} else {
				check(args[m[2]], field, "unknown or unsupported prompt argument: "+m[2])
			}
		}
	}
	tokens(g.Instructions, "instructions", nil)
	identities := map[string]bool{}
	for _, c := range in.Catalog {
		identities[c.Kind+":"+config.CatalogName(c.Upstream, c.Kind, c.Name, c.Alias)] = true
	}
	identity := func(kind, name, field string) {
		check(!identities[kind+":"+name], field, "exposed catalog identity collides with another selection")
		identities[kind+":"+name] = true
	}
	resourceNames := map[string]bool{}
	promptNames := map[string]bool{}
	for i, r := range g.Resources {
		p := fmt.Sprintf("resources/%d", i)
		check(ValidSlug(r.Name) && !resourceNames[r.Name], p+"/name", "must be a unique resource slug")
		resourceNames[r.Name] = true
		size(r.Title, 120, p+"/title")
		size(r.Description, 20000, p+"/description")
		size(r.Text, 20000, p+"/text")
		check(r.MimeType == "text/plain" || r.MimeType == "text/markdown", p+"/mimeType", "must be text/plain or text/markdown")
		tokens(r.Text, p+"/text", nil)
		identity("resource", config.LocalResourceURI(r.Name), p+"/name")
	}
	for i, prompt := range g.Prompts {
		p := fmt.Sprintf("prompts/%d", i)
		check(ValidSlug(prompt.Name) && !promptNames[prompt.Name], p+"/name", "must be a unique prompt slug")
		promptNames[prompt.Name] = true
		size(prompt.Title, 120, p+"/title")
		size(prompt.Description, 20000, p+"/description")
		check(len(prompt.Arguments) <= 20, p+"/arguments", "at most 20 arguments")
		check(len(prompt.Messages) > 0 && len(prompt.Messages) <= 20, p+"/messages", "provide 1–20 text messages")
		args := map[string]bool{}
		for j, a := range prompt.Arguments {
			q := fmt.Sprintf("%s/arguments/%d", p, j)
			check(guidanceKey.MatchString(a.Name) && !args[a.Name], q+"/name", "must be a unique argument name")
			args[a.Name] = true
			size(a.Description, 1000, q+"/description")
		}
		total := 0
		for j, m := range prompt.Messages {
			q := fmt.Sprintf("%s/messages/%d", p, j)
			check(m.Role == "user" || m.Role == "assistant", q+"/role", "must be user or assistant")
			total += utf8.RuneCountInString(m.Text)
			tokens(m.Text, q+"/text", args)
		}
		check(total <= 20000, p+"/messages", "message text must total at most 20000 characters")
		identity("prompt", config.LocalPromptName(prompt.Name), p+"/name")
	}
	seen := map[string]bool{}
	for i, slug := range g.UpstreamInstructions {
		p := fmt.Sprintf("upstreamInstructions/%d", i)
		check(ValidSlug(slug) && !seen[slug], p, "must be a unique upstream slug")
		seen[slug] = true
		identity("resource", config.UpstreamInstructionsURI(slug), p)
	}
	return errs
}
func (t *Tx) validateGuidanceSources(ctx context.Context, g *config.Guidance, previous *config.Guidance) error {
	if g == nil || len(g.UpstreamInstructions) == 0 {
		return nil
	}
	rows, err := t.q.Query(ctx, `SELECT slug FROM upstreams WHERE tenant_id=`+tenantSQL+` AND slug=ANY($1::text[])`, g.UpstreamInstructions)
	if err != nil {
		return err
	}
	defer rows.Close()
	found := map[string]bool{}
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return err
		}
		found[s] = true
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for i, s := range g.UpstreamInstructions {
		if !found[s] && (previous == nil || !slices.Contains(previous.UpstreamInstructions, s)) {
			return apierror.Validation([]apierror.FieldError{vf(fmt.Sprintf("/guidance/upstreamInstructions/%d", i), "unknown upstream")})
		}
	}
	return nil
}
