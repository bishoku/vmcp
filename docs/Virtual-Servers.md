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
- At most 2000 characters. Only the description is overridden; input schemas are passed through unchanged.

## Ordering and the UI

The Virtual Servers page lists saved servers in a searchable table, with capability
counts, authentication, rate limits and status. Select a server to open its
read-only detail page. Overview, Capabilities, Access, Protection, Connect and
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
