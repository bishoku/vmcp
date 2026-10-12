# Upstreams

An **upstream** is a real MCP server vMCP connects to.

| Field | Meaning |
|---|---|
| `slug` | Identifier in URLs and tool names (`[a-z0-9-]`) |
| `name`, `description` | Display text |
| `url` | Endpoint, `http://` or `https://` |
| `fallbackUrls` | Up to 5 more URLs of the same server, tried in order ([failover](Resilience-and-HA.md)) |
| `transport` | `streamable_http` (default), `sse` (legacy HTTP+SSE), or `stdio_worker` (separate mTLS owner endpoint) |
| `enabled` | Disabled upstreams are not served |
| `headers` | Sent on every upstream request, typically `Authorization`. **Encrypted at rest, write-only, never returned**; the API only lists `headerNames` |
| `tls` | Custom CA, server name override, mTLS ([Upstream TLS](Upstream-TLS.md)) |

## View and edit in the web UI

The Upstreams table opens a dedicated page at `/upstreams/<slug>`. Saved
configuration opens read-only, with node health and stored discovery snapshots.
Choose **Edit upstream** at the top to change connection settings, credentials,
TLS or OAuth. **Save** writes the versioned configuration and returns to read-only
mode; **Cancel** restores saved values after confirming any unsaved changes.
Viewers cannot enter edit mode. **Add upstream** opens a full-page creation form.

**Test stored credentials** uses the saved configuration. In edit mode,
**Test edited connection** uses the draft and only credentials entered in this
session; stored secret values remain write-only. Reloading discovery snapshots
reads the cache. Use **Refresh capabilities** in the upstream table for fresh
discovery. Existing `?inspect=<slug>` links redirect to the dedicated detail page.

## Discovering tools and catalogs

`POST /api/v1/upstreams/{slug}/refresh-tools` connects, lists the tools and caches them. The response reports
`added`, `removed`, `changed` and **orphaned selections** (virtual-server entries whose upstream tool vanished).
`POST /api/v1/connection-tests` tries a connection without saving anything (the UI's "Test connection").

Tool schemas are cached, so virtual servers keep working while an upstream is briefly down. Refresh when an upstream
changes its tools, resources or prompts. Discovery also caches advertised resources,
URI templates, prompts and capability metadata. See [catalogs](MCP_CATALOGS.md).
A saved upstream can be tested with `POST /api/v1/upstreams/{slug}/connection-test`
without returning its credentials. Stdio uses operator-approved immutable images
on a [separate worker](TENANT_AND_STDIO_OPERATIONS.md), with no fallback URLs.

## Editing headers safely

`PUT` with `headers` omitted keeps the stored headers; `{}` clears them. With `PATCH`, a header set to `null` is deleted
and others are merged. Exported configuration never contains header values.

## Deleting

Deleting an upstream used by virtual servers returns `409` unless you pass `?force=true`, which also removes those
selections.
