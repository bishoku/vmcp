# Virtual servers

A **virtual server** is an endpoint, `/mcp/v1/virtual/{slug}`, that exposes curated upstream tools, resources, URI templates and prompts. Agents
connect to it like to any MCP server.

| Field | Meaning |
|---|---|
| `slug`, `name`, `description` | Identification |
| `enabled` | Disabled servers are not served |
| `authRequired` | Default `true`: callers need an [agent key](Agent-Keys-and-Authentication.md) permitted for this server |
| `tools` | Ordered selections: `{upstream, tool, alias?, description?}` |
| `catalog` | Resource/template/prompt selections: `{upstream, kind, name, alias?, description?}`; [routing rules](MCP_CATALOGS.md) |
| `clientFeatures` | Opt-in sampling, roots and form elicitation permissions; [limits and host consent](MCP_CLIENT_FEATURES.md) |
| `policy` | [Tool policy](Tool-Policies.md) |
| `pii` | [PII masking](PII-Masking.md) |
| `rateLimitPerMinute` | [Rate limit](Rate-Limits.md) |

## Naming

An exposed tool is named `{upstream}__{tool}` unless you set an `alias`, which lets you resolve clashes (two upstreams
with an `echo` tool) or give agents friendlier names. Exposed names must be unique within a virtual server; the API and
UI report duplicates.

## Overriding descriptions

`description` replaces the upstream description **in `tools/list` of this virtual server only**. The same upstream tool
can be described differently elsewhere, which helps agents choose between look-alike tools.

- Empty or omitted: the upstream text is used.
- Reads return `upstreamDescription` (current upstream text) and `descriptionDrift`: `true` when the upstream text
  changed after the override was written. The UI shows the upstream text and a warning. Saving the selection again
  acknowledges the change (even a repeated apply of the same document does).
- At most 20,000 Unicode characters (also applies to resource, template and prompt description overrides). Only the description is overridden; input schemas are passed through unchanged.

## Ordering and the UI

The Virtual Servers page lists saved servers in a searchable table, with capability
counts, authentication, rate limits and status. Select a server to open its
read-only detail page. Overview, Capabilities, Guidance, Access, Protection, Connect and
Activity remain available without entering edit mode.

Choose **Edit configuration** at the top to open the full-page editor at
`/virtual-servers/<slug>/edit`. Order of `tools` is the order agents see. In the
editor, drag tools to reorder, type aliases and descriptions, and set per-tool
policies. **Save** returns to the saved detail page; **Cancel** returns without
applying changes and prompts before discarding a dirty draft. Viewers cannot edit.
**New virtual server** opens a dedicated creation page. Existing `?inspect=<slug>`
links redirect to read-only details.

## Orphans and disabled upstreams

If an upstream tool disappears after a refresh the selection is reported as orphaned and is not served. Tools of a
disabled upstream are skipped.

## Example

```json
{
  "name": "Support agent",
  "authRequired": true,
  "tools": [
    {"upstream": "crm", "tool": "search_customers", "alias": "find_customer",
     "description": "Look up a customer by e-mail or phone. Read-only."},
    {"upstream": "tickets", "tool": "create_ticket"}
  ],
  "policy": {"default": "allow", "tools": {"create_ticket": "deny"}},
  "pii": {"enabled": true, "detectors": ["email", "phone"]},
  "rateLimitPerMinute": 120
}
```

### Editing long descriptions

Selected capabilities show a compact description summary. **Edit description** opens
an editor with the full plain text, the current upstream source, and an agent view
of the effective description and exposed name. Line breaks and spacing are preserved.
The agent view previews configuration before any configured PII masking.

**Apply to draft** updates the virtual server form only; use the existing server
save action to publish changes. Cancel leaves the form unchanged. **Use upstream
description** clears the override in the editor; apply and save to publish that
change. Upstream descriptions are read-only and are not limited by the override
editor's 20,000-character limit. The existing 1 MiB control-plane request budget
still applies to the complete server update. Capabilities on the detail page can be inspected
without entering edit mode.

On desktop screens (1,024 px and wider), **Compare** shows the current upstream
and the override draft side by side. Removed and added lines have both color and
minus/plus indicators; whitespace and line breaks are preserved. This compares
against the current source, not historical upstream revisions. An empty override
inherits the source and therefore has no effective differences. Very large changed
sections are grouped into removed/added blocks to keep comparison work bounded.
Comparison is unavailable on smaller screens; resizing to mobile switches to the
agent view without changing the draft.

## Agent guidance

The **Guidance** detail tab and the **Agent guidance** editor panel manage content
owned by a virtual server. This is additive: selected upstream resources, resource
templates, prompts, description overrides and tool routing continue to work as before.
Enabled local resources and prompts also appear in the Capabilities detail list.

- **Server instructions** are returned in the MCP initialization response. They
  provide a short introduction, not an enforced system prompt. Clients need to
  initialize again to receive changed instructions.
- **Local resources** publish plain text or Markdown at
  `vmcp://guidance/resource/<name>`. Agents discover them with `resources/list`
  and read them with `resources/read`.
- **Local prompts** publish as `vmcp_local__<name>` through `prompts/list` and
  `prompts/get`. Each prompt contains ordered user/assistant text messages and
  optional or required string arguments. Optional omitted arguments become empty
  text. Unknown arguments and missing required arguments are rejected.
- **Upstream instructions** are cached from discovery's initialization response.
  Explicitly selected sources publish separate resources at
  `vmcp://guidance/upstream/<upstream-slug>`. They are not merged into server
  instructions. An unavailable/disabled upstream or empty discovered instructions
  produces no resource; refresh discovery to update cached source text.

Clients decide whether and when to consume resources and prompts. Guidance does
not execute tools, enforce a call order, fetch external content, or grant access.
Local resource templates, executable workflows, template expressions/conditionals,
argument completion and resource subscriptions are outside this first version.
Existing upstream template/completion/subscription support is unchanged.

### Editing and references

Long content uses compact summaries and a wide editor. **Apply to draft** changes
only the draft; the virtual server's main **Save** publishes all changes together.
Cancel/close asks before discarding edited content. Read-only detail views provide
content inspection and prompt preview with sample argument values. Preview performs
literal substitution only, before runtime policy/PII processing; it makes no MCP calls.

Define a stable tool reference by key, upstream slug and original tool name, then
write `{{tool:key}}` in instructions, resource text or prompt messages. The snapshot
resolves the current exposed name, including aliases. Prompt messages additionally
support `{{arg:name}}`. Substituted argument text is never evaluated as another
placeholder. Other text is preserved verbatim; there is no general template engine.

A referenced tool must remain selected when saving. If discovery, upstream disablement
or policy later makes it unavailable, dependent resources/prompts are omitted and
instructions with unavailable references are withheld. Independent local content
continues to work. Tool references and upstream sources are scoped to the current tenant.

### API and limits

The existing OpenAPI virtual-server POST, GET, PUT and PATCH operations accept a
`guidance` object. The individual GET returns full content; inventory lists omit
that payload and expose `hasGuidance`. Existing version/ETag concurrency checks,
admin authorization, change auditing and export/apply apply to guidance too.

Omitting `guidance` on PUT preserves saved content for older clients; supplying
`{}` clears it. PATCH replaces only supplied non-null guidance members; arrays
replace in full. For example, `{"guidance":{"instructions":"New introduction"}}`
keeps resources/prompts, while `{"guidance":{"resources":[]}}` clears only resources.
An omitted/null member is preserved on PATCH. Export/apply round-trips full guidance.

Limits use Unicode character counts: 4,000 for server instructions; 20,000 per
resource body or description; 20,000 combined message characters per prompt;
120 for titles; 1,000 for argument descriptions. Each virtual server allows up to
100 resources, prompts, tool references and selected instruction sources; each
prompt allows up to 20 arguments and 20 messages. The guidance document must fit
in 512 KiB, within the existing 1 MiB complete API request budget. Rendered prompt
messages are bounded to 100,000 characters in total. Local identities are validated
for collisions with selected upstream identities; no implicit shadowing occurs.

Example `guidance` member for a server selecting `crm/search_customers`:

```json
{
  "instructions": "Read vmcp://guidance/resource/workflow before using {{tool:find}}.",
  "toolReferences": [{"key":"find","upstream":"crm","tool":"search_customers"}],
  "resources": [{
    "name":"workflow", "title":"Customer workflow", "description":"Safe lookup sequence",
    "enabled":true, "mimeType":"text/markdown",
    "text":"1. Call {{tool:find}}.\n2. Ask for clarification if multiple customers match."
  }],
  "prompts": [{
    "name":"customer-review", "title":"Review customer", "description":"Prepare a customer review",
    "enabled":true,
    "arguments":[{"name":"customer","description":"Customer to review","required":true}],
    "messages":[{"role":"user","text":"Review {{arg:customer}} using {{tool:find}} and follow the workflow resource."}]
  }],
  "upstreamInstructions":["crm"]
}
```

Serving local content uses the immutable snapshot and prepared catalog indexes;
no content lookup is added to PostgreSQL on each read/render. Existing session
authorization/revocation, rate limiting, approval, PII masking and audit admission
and outcome handling remain on the request path. Disabling a virtual server or
revoking its credentials also protects local capabilities. Guidance is included
in migration `00027_guidance.sql`; apply migrations before starting the new binaries.

The editor groups agent guidance and access/protection settings into expandable
sections while keeping server identity and capability selection directly
accessible. Summary cards use the same layout as upstream and virtual-server
saved details. Expanding or collapsing a section does not discard draft changes;
validation links open the relevant section before focusing its field.
