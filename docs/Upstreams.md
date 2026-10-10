# Upstreams

An **upstream** is a real MCP server vMCP connects to.

| Field | Meaning |
|---|---|
| `slug` | Identifier in URLs and tool names (`[a-z0-9-]`) |
| `name`, `description` | Display text |
| `url` | Endpoint, `http://` or `https://` |
| `fallbackUrls` | Up to 5 more URLs of the same server, tried in order ([failover](Resilience-and-HA.md)) |
| `transport` | `streamable_http` (default) or `sse` (legacy HTTP+SSE) |
| `enabled` | Disabled upstreams are not served |
| `headers` | Sent on every upstream request, typically `Authorization`. **Encrypted at rest, write-only, never returned**; the API only lists `headerNames` |
| `tls` | Custom CA, server name override, mTLS ([Upstream TLS](Upstream-TLS.md)) |

## Discovering tools

`POST /api/v1/upstreams/{slug}/refresh-tools` connects, lists the tools and caches them. The response reports
`added`, `removed`, `changed` and **orphaned selections** (virtual-server entries whose upstream tool vanished).
`POST /api/v1/connection-tests` tries a connection without saving anything (the UI's "Test connection").

Tool schemas are cached, so virtual servers keep working while an upstream is briefly down. Refresh when an upstream
changes its tools.

## Editing headers safely

`PUT` with `headers` omitted keeps the stored headers; `{}` clears them. With `PATCH`, a header set to `null` is deleted
and others are merged. Exported configuration never contains header values.

## Deleting

Deleting an upstream used by virtual servers returns `409` unless you pass `?force=true`, which also removes those
selections.
