# Upstreams

An **upstream** is an MCP server or a REST API that vMCP exposes through virtual servers.
The connection/discovery instructions below apply to MCP servers; see [REST API upstreams](#rest-api-upstreams) for REST configuration.

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
without returning its credentials.

## Editing headers safely

`PUT` with `headers` omitted keeps the stored headers; `{}` clears them. With `PATCH`, a header set to `null` is deleted
and others are merged. Exported configuration never contains header values.

## Deleting

Deleting an upstream used by virtual servers returns `409` unless you pass `?force=true`, which also removes those
selections.

## HTTP-only upgrade

Only HTTP endpoints are supported. Before upgrading an existing deployment, remove
any upstream whose transport is neither `streamable_http` nor `sse`, or replace it
with an actual HTTP MCP endpoint. Remove or update its virtual-server selections
as needed. Migration 26 enforces the HTTP-only database constraint and refuses to
proceed while unsupported configurations exist; it does not delete configurations.
Migration version 24 is retained as a no-op for migration-history compatibility.
Run the migration command before restarting the control and data planes.

## REST API upstreams

Choose **REST API** when creating an upstream to publish existing JSON HTTP services
as MCP tools. Existing upstreams remain MCP servers; `kind` cannot change after creation.
REST operations participate in the same virtual-server selection, aliases, description
overrides, tool policies, approvals, rate limits, PII handling and durable audit flow.
Local virtual-server guidance can refer to them like other tools.

In the saved detail page, **Edit upstream** exposes REST authentication, request controls
and operations. Each enabled operation defines a tool name, description (up to 20,000
characters), method, path and schemas. Operation and long-text dialogs update the draft;
only the main **Save** publishes it. Saving never calls the REST service. The tool catalog
and configuration are updated in one transaction. Disabling/removing an operation makes
existing selections orphaned and unavailable after configuration propagation.

### Requests and responses

- Methods: GET, POST, PUT, PATCH and DELETE. Paths append to the base URL, e.g.
  `https://internal.example/api` + `/orders/{id}`. No dynamic hosts, traversal,
  redirects or fallback URLs are supported.
- Named path/query parameters have scalar JSON schemas (`string`, `integer`, `number`,
  `boolean`). Path parameters are always required. Values are validated and URL encoded.
- An optional request-body schema adds a required tool argument named `body` and sends
  it as JSON. GET bodies are unsupported. Other input fields are rejected.
- Responses must be JSON (204 maps to `null`), at most 1 MiB, and match the configured
  response schema. Optional response-field selection keeps declared top-level object fields.
  MCP output is an object `{ "result": ... }`, returned as structured content and JSON text.
- Schemas are inline JSON Schema 2020-12; schema references, multipart uploads, binary
  responses, streaming and pagination orchestration are outside this version.
  OpenAPI/Swagger import can expand compatible local references before saving (see below).
- Success defaults to any 2xx response; an optional allowlist narrows it. Error responses
  expose status and a sanitized message, never raw backend bodies or credentials.
- The total request timeout (including token acquisition) is 1–120 seconds, default 30.
  vMCP does not replay failed REST operations, including lost responses on pooled
  HTTP/1.1 or HTTP/2 connections and 401 responses. Operator-supplied idempotency
  headers do not enable transport replay. A timeout does
  not prove that a write failed; verify the service state before a user-initiated retry.

### Authentication, headers and identity

`rest.auth.type` supports `none`, `basic`, `bearer` and `oauth2`. OAuth uses the client
credentials grant, optional scopes/audience, and explicit `client_secret_basic` or
`client_secret_post` token authentication. Tokens are cached and shared per credential
configuration on each node. Token endpoints use system TLS trust; upstream custom CA
and mTLS settings apply to the REST service itself. Agent credentials are never reused
as service credentials.

REST configuration, authentication secrets and operation header values are encrypted at
rest. API reads and exports include secret-presence flags and header names only. Omitted
or empty auth secrets retain existing values when the auth type stays the same; changing
the type to `none` clears managed authentication. A supplied `rest` block replaces its
settings/operation list; omitting the whole block preserves it. Operation headers are
preserved by operation name when omitted, replaced when supplied, and cleared by `{}`.
Common upstream headers retain the existing PUT/PATCH semantics described above.

Common and operation-level manual headers support API keys and other service metadata.
Operation headers override common headers case-insensitively. Managed authentication
cannot be combined with a manual `Authorization` header. Transport-controlled headers
and all `X-VMCP-*` headers are reserved. Never put secrets in tool descriptions, schemas,
paths or query parameters: those are not secret fields and can be visible to agents/operators.

Enable `forwardIdentity` to send `X-VMCP-Agent-Id`, `X-VMCP-Auth-Type` and
`X-VMCP-Virtual-Server` from the authenticated vMCP session. Agent-key callers use the
immutable key record ID; OAuth callers use a stable SHA-256 digest scoped by tenant,
issuer and subject. Caller-supplied headers, tool arguments and MCP clientInfo cannot
choose these values. Anonymous calls omit identity headers; `requireIdentity` rejects
them before dispatch. These headers are trusted gateway metadata, not delegated user
authorization: the REST service must authenticate vMCP and restrict direct access before
relying on them.

HTTPS is required for both service and token endpoint. `allowInsecureHttp` is an explicit
exception for trusted development networks. Existing egress allowlists, DNS/IP checks,
proxy restrictions and TLS verification still apply, including for OAuth token requests.

### Preview, explicit testing and API automation

`POST /api/v1/upstreams/{slug}/rest/preview` validates arguments and returns the prepared
request with masked header values. It performs no network request. Editor access is required.
`POST /api/v1/upstreams/{slug}/rest/test` executes a real operation with saved credentials;
it requires admin access, an enabled upstream and matching `If-Match`. The UI asks for
confirmation because the operation may change service data. Administrative tests do not
impersonate an agent; identity-required operations must be tested through an authenticated
MCP client. Both endpoints accept `{ "operation": "lookup", "arguments": { "id": "42" } }`.

REST endpoints are never automatically contacted by discovery or health probes. MCP
connection-test/refresh endpoints reject REST upstreams. The recorded health indicator
therefore does not imply REST availability. Runtime calls use the common circuit breaker,
metrics and audit pipeline.

Create/update REST upstreams through the existing upstream CRUD and export/apply APIs.
The canonical OpenAPI document describes all REST fields and endpoints. For example:

```json
{
  "slug": "orders",
  "name": "Orders API",
  "kind": "rest",
  "url": "https://orders.internal/api",
  "rest": {
    "auth": { "type": "basic", "username": "vmcp", "password": "PROVISION_SECRET_HERE" },
    "timeoutSeconds": 30,
    "forwardIdentity": true,
    "requireIdentity": true,
    "operations": [{
      "name": "get_order",
      "description": "Retrieve one order by its identifier.",
      "enabled": true,
      "method": "GET",
      "path": "/orders/{id}",
      "readOnly": true,
      "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }],
      "responseSchema": { "type": "object" }
    }]
  }
}
```

Run migration **28** before deploying the updated control/data planes. It adds the kind
and encrypted REST configuration columns while preserving existing MCP records. Deploy
all nodes with REST support before configuring REST upstreams; older data planes do not
understand this upstream kind. Existing secret-key rotation includes REST configuration.
Snapshot refreshes reuse immutable prepared REST schemas when the decrypted
configuration, base URL and common headers are unchanged. Changed definitions are
revalidated; snapshot freshness, authenticated decryption and authorization checks
remain enforced.

### Import OpenAPI / Swagger definitions

In a REST upstream draft, choose **Import OpenAPI / Swagger** and upload a local
JSON/YAML file or paste its contents. **Analyze definition** lists the operations;
nothing is selected, saved or executed automatically. Search by method, path, tag
or tool name, select the operations to publish, and choose **Add to draft**. Review
or edit the imported operations, then use the upstream's main **Save** action.
Canceling the import or upstream draft does not change saved configuration.

The import preview is also available to editors/admins through
`POST /api/v1/rest-imports` with `{ "document": "<complete JSON or YAML text>" }`.
The response contains `title`, `version`, informational `servers`, `warnings` and
`operations`. Each candidate has a stable method/path `id`, `summary`, `tags`,
`warnings`, `errors` and, when compatible, an `operation` in the regular REST format.
Choose candidates explicitly and submit them with the existing upstream CRUD API.
The preview creates no upstreams and does not change the configuration revision.

Supported source formats are Swagger 2.0 and OpenAPI 3.0/3.1. Local `#/...`
references are expanded, inherited path parameters are merged with operation
parameters, and operation IDs become normalized tool names. Duplicate names receive
suffixes. In the UI, collisions with current tool names also receive new names;
existing operations are never replaced. An already configured method/path cannot be
added again through the importer. The 200-operation upstream limit still applies.

The importer preserves authentication, manual headers, TLS, HTTP exceptions and
agent identity settings. Declared servers are shown for manual comparison with the
configured base URL. Declared security requirements produce a review warning;
token endpoints, credentials and security schemes are never applied automatically.
GET operations remain conservatively marked `readOnly: false` until reviewed.
Examples and defaults are omitted from generated schemas. Read-only request
properties and their required entries are removed; write-only response fields
require manual filtering and are reported as incompatible.

Unsupported operations remain visible with a reason and cannot be selected. This
includes external/cyclic references, operation/path-specific servers, multipart or
binary bodies, dynamic header/cookie/form parameters, unsupported serialization,
and success responses without JSON content (except 204). JSON bodies use
`application/json`. Only explicit 2xx statuses are imported; `default` and range
responses alone require manual configuration. Multiple success schemas become an
`anyOf`. Optional request bodies receive an explicit warning because this REST
adapter exposes its body argument as required. OpenAPI 3.0 nullable types and boolean
exclusive bounds are converted to inline JSON Schema; advanced reference siblings,
schema identifiers and discriminators require manual review.

The source limit is 2 MiB, with at most 1,000 source operations and an 8 MiB preview.
Nesting and reference expansion are bounded. Multiple YAML documents, aliases,
custom tags and duplicate keys are rejected. The importer never fetches a URL or
reads referenced files. Existing REST save-time validation and all runtime security
controls still apply to imported operations. No additional database migration is
needed beyond REST upstream migration 28.

Format references: [OpenAPI 3.0](https://spec.openapis.org/oas/v3.0.4.html),
[OpenAPI 3.1](https://spec.openapis.org/oas/v3.1.0.html),
[Swagger 2.0](https://spec.openapis.org/oas/v2.0.html).
