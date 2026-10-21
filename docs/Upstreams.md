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

In the saved detail page, **Edit upstream** opens the **REST → MCP workspace**.
Select an operation on the left, edit its request and tool information in the center,
and inspect the live **MCP preview** on the right. On narrower screens, use **Editor**
and **MCP preview** to switch views. Import and operation editing stay on the same page.

Each enabled operation defines a tool name, description (up to 20,000 characters),
method, path and schemas. Entering `/orders/{id}` automatically creates a required path
parameter. Visual parameter rows edit names, scalar types, descriptions and required
flags; **Add query parameter** adds filters or options. Existing imported schema
constraints are preserved. **Advanced · body, response & headers** exposes parameter
JSON editing, inline JSON schemas, response field selection, success statuses and
write-only operation headers.
Invalid JSON must be corrected or explicitly discarded before switching operations.

**Connection & authentication** contains the base URL, identifier, service credentials
and request controls. **Advanced connection settings** contains TLS and shared headers.
Edits update the upstream draft; only the main **Save** updates the saved catalog.
Saving never calls the REST service. The tool catalog and configuration are updated
in one transaction. Select tools separately in a virtual server to expose them to agents.
Disabling/removing an operation makes existing selections orphaned and unavailable
after configuration propagation.

The live MCP preview contains input/output schemas (including the `result` wrapper)
and tool annotations, but no authentication or request-header values. It is a local
draft preview, not a service test or a substitute for save-time backend validation.
Real request previews and execution tests still use the saved configuration.

### Diagnosing REST connection failures

Data-plane `upstream call failed` logs include a safe `reason`, `detail`, execution
`stage`, `duration_ms`, `virtual_server`, `request_id` and `trace_id`. For example,
`egress_host_not_allowed` means the destination must be included in
`VMCP_EGRESS_ALLOW_HOSTS`. When that variable is nonempty, public hosts also require
an explicit entry. Private/loopback addresses additionally require an allowed CIDR.
Retain existing host entries when adding a service, and recreate containers after
changing their environment; restarting an existing container does not update it.

Other reasons distinguish DNS failure, timeout, certificate verification failure,
connection refusal/reset and prohibited destinations. Diagnostics never log the
raw transport error, request URL/query, headers, arguments or response body.
Agent-facing transport errors remain generic. A successful HTTP status on the
MCP POST does not imply a successful tool call; inspect its JSON-RPC result/error.

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
or tool name, select the operations to include, and choose **Add to draft**. Selecting
an operation or choosing its **Preview** action displays its MCP definition in the
workspace preview panel. Review
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

### Build REST schemas visually

The REST workspace includes visual request-body and response schema editors.
Paste example JSON and choose **Generate request body schema** or **Generate
response schema**, then edit types, descriptions and required fields. Objects
expand into nested fields; arrays expose their item schema. GET operations do
not show a request-body editor. Example values remain local and are not saved
with the upstream.

Inferred fields start optional. Null values, empty arrays and mixed-type arrays
need an explicit type choice. Examples supplement existing definitions without
replacing imported constraints, required fields or scalar types. Closed objects
(`additionalProperties: false`) do not gain fields from examples. Complex schema
sections remain available through the advanced JSON editor; constrained types
are locked in the visual editor to avoid silently dropping imported rules.

For a saved upstream, **Execute test request** uses the saved configuration and
credentials, even while editing a draft. After a successful test, **Use test
response as schema** adds schema information to the matching draft operation.
It uses the returned result, including any saved response-field projection, not
the MCP envelope. Failed results cannot be used. Save the draft to publish the
schema. New operations must first be saved before they can be tested; execution
still requires administrator access and explicit confirmation.

### Diagnose REST request-body mapping

The request-body schema describes the **HTTP body**, not the complete MCP
arguments object. The tool exposes that schema under a required `body` argument.
For example, if the order service expects this JSON:

```json
{"order":{"items":[{"productId":"SKU-1","quantity":2}]}}
```

the MCP call uses:

```json
{
  "name": "orders__create_order",
  "arguments": {
    "body": {"order":{"items":[{"productId":"SKU-1","quantity":2}]}}
  }
}
```

Use the actual exposed tool name from `tools/list`. vMCP validates the arguments,
takes `arguments.body`, and JSON-encodes that value as the HTTP request body.
It does not send the outer `body` wrapper, flatten nested objects, rename keys,
convert strings into numbers, or insert schema defaults. Objects, arrays, null,
false, zero, and empty arrays retain their JSON types. If the service expects
`{"items":[...]}` instead, omit the `order` level from both the example schema
and `arguments.body`. A body that is itself an array is also supported.

Path and query parameters are separate top-level MCP arguments. For example,
`{"id":"ORD-42","notify":false,"body":{"order":{"items":[]}}}` can become
`PUT /orders/ORD-42?notify=false` with only `{"order":{"items":[]}}` in the body.
Path/query parameters support scalar types; JSON body fields support nested types.

Virtual-server **PII argument masking** is a separate step before REST validation
and dispatch: when enabled, it intentionally replaces matching argument values.
This can make customer/product identifiers invalid for the service. Saved-upstream
preview does not apply a virtual server's PII policy. Audit payload masking only
changes recorded evidence and does not change the request sent upstream.

| Observation | Interpretation |
| --- | --- |
| `arguments do not match the operation input schema` | Rejected before HTTP dispatch. Check the required `body` wrapper, exact field names, types, and nested `required` constraints against `tools/list`. The current message does not identify the failing field. |
| `REST service returned HTTP 400/422` | The request reached the service, which rejected it. Example-inferred fields are optional until marked required; missing business fields can pass the gateway's schema. |
| `REST service returned HTTP 201` | The endpoint responded with 201, but the configured success-status list excluded it. An empty list accepts all 2xx responses. The operation may already have succeeded. |
| `REST response does not match the configured schema` | The request was sent; response validation failed. Configure the schema for the raw service response, not the MCP result envelope. A 204 response is validated as null. |
| `REST service returned invalid JSON` | The request was sent, but a non-204 response was not valid JSON, including an empty 201 response. The operation may already have succeeded. |

Use **Preview request** on the saved upstream with the same MCP arguments to
inspect the URL and body without executing the operation. Save draft schema
changes first, and refresh the client's `tools/list` after configuration updates.
With upstream audit capture enabled, inspect the request and response records
to compare the constructed body and HTTP status; captured data is masked and
bounded, not a byte-for-byte substitute for the service's own request trace.
Do not blindly retry a create operation after a response-validation failure:
verify whether the service created the order first.

### Select the fields returned to agents

Use **Fields returned to the agent** below the response schema editor. **Full
response** keeps the original response, including undeclared fields. **Selected
fields only** starts with the declared top-level properties; uncheck those the
agent should not receive. Keep at least one selection. Selecting an object or
array returns its entire contents; nested property selection is not supported.

The MCP preview immediately reflects the selected properties and their required
flags. Save applies the same projection to both structured and text tool output;
unselected and undeclared properties are omitted. The original response schema
remains intact and validates the upstream response before projection. Optional
selected properties absent from the response remain absent; null values are
preserved. If a selected property is removed from the schema, resolve the
selection error before saving. Choosing all declared fields still filters out
undeclared fields, unlike **Full response**.

### Per-call caller context

Upstream **Caller context** settings optionally forward selected string fields from
`tools/call` → `params._meta`. This is caller-supplied application context, not a
verified vMCP identity. The agent backend supplies a current token on every call;
the upstream validates it and enforces its own authorization. vMCP does not exchange,
refresh, or validate these tokens. vMCP authentication remains separate.

Example upstream configuration (REST):

```json
{
  "callerContext": {
    "enabled": true,
    "metaKey": "com.bank/caller-context",
    "fields": [
      {"name": "token", "header": "X-Bank-User-Token", "required": true},
      {"name": "chatSessionId", "header": "X-Chat-Session-Id"}
    ]
  }
}
```

The agent sends:

```json
{
  "name": "getProducts",
  "arguments": {},
  "_meta": {
    "com.bank/caller-context": {
      "token": "example-upstream-token",
      "chatSessionId": "chat-456"
    }
  }
}
```

For MCP upstreams, omit `header` and `bearer` from the field definitions. Selected
fields keep their namespace in outgoing tool metadata; unrelated fields are not
forwarded. Gateway-managed progress correlation is preserved. Both Streamable HTTP
and legacy SSE are supported. Context is not added to discovery or initialization.

For REST, a field may target `Authorization` with `"bearer": true` to prepend
`Bearer `. Managed REST authentication must then be `none`, and neither common nor
operation-level static headers may configure the same header. Header matching is
case-insensitive. Transport-managed and `X-VMCP-*` headers cannot be targets.
REST redirects are not followed. All configured MCP fallback URLs belong to the
same trusted upstream and may receive the selected metadata after a fallback.

The feature is disabled by default. Rules apply to every tool call to the selected
upstream. Missing optional or empty optional values are omitted; missing/empty
required fields and invalid supplied values reject the call before dispatch.
Fields must be strings, at most 8 KiB (including an optional Bearer prefix), without
CR, LF or NUL; total incoming metadata is limited to 32 KiB when enabled. At most 16
fields may be configured. Values are request-scoped, not cached on a session or
shared HTTP client. Chat session IDs are distinct from MCP session IDs.

API create, PUT, PATCH, and declarative apply/export support `callerContext`.
Omission preserves saved settings; `{}` clears them. The entire supplied block
replaces previous rules. The editor stores mapping rules, never runtime tokens.
REST preview shows mapping placeholders. Admin execution tests cannot supply caller
context; test required fields using an actual MCP tool call instead.

Apply migration **30 (`00030_caller_context.sql`)** before starting the updated
services. Existing upstreams default to disabled. Audit/log masking behavior is
unchanged by this feature; it does not guarantee exclusion of caller tokens from
existing recording paths. Configure and evaluate those policies separately.

## Per-call request header mappings

Use **Per-call context → Configure mappings** in an upstream's edit screen to
forward selected HTTP headers from an agent's `tools/call` request. This works
with REST, Streamable HTTP MCP, and legacy SSE MCP upstreams. A mapping copies
values unchanged; using the same source and target name forwards the header
without renaming it.

```json
{
  "requestHeaderMappings": [
    {"source": "X-Agent-Token", "target": "X-Customer-Token", "required": true},
    {"source": "X-Chat-Session-Id", "target": "X-Session-Id"}
  ]
}
```

Mappings are an upstream setting, separate from `callerContext` metadata rules.
Only listed headers reach the selected upstream. Missing optional headers are
omitted; missing or empty required headers fail the tool call before dispatch.
Names are case-insensitive. Multiple values are preserved. Each mapped header is
limited to 8 KiB of values, and all mapped values together to 32 KiB; at most 16
rules are allowed. Control characters other than horizontal tabs are rejected.

Source and target names cannot be transport, session, gateway authentication,
or proxy identity headers (including `Authorization`, `Cookie`, `Mcp-*`,
`X-VMCP-*`, and `X-Forwarded-*`). Use a dedicated application header for a token
that the upstream validates. Mapping targets must be unique and cannot collide
with static upstream/operation headers or enabled REST caller-context mappings.
Headers named by the incoming `Connection` header cannot be mapped either.

Values belong to the current tool invocation; they are not stored in the MCP
session, added to shared clients, or sent with initialize, discovery, background
SSE connections, or other MCP methods. Retry/fallback attempts for that invocation
use the same values; all configured fallback endpoints must therefore belong to
the same trusted service. The upstream remains responsible for token validation.
Audit/log capture behavior is unchanged.

The API accepts `requestHeaderMappings` on create, PUT, PATCH, and declarative
apply/export. Omit it to preserve stored rules; send `[]` to clear them. The
configuration contains names and required flags only, never runtime values.
REST request previews show placeholders for mapped headers. Administrative
execution tests do not forward the administrator's headers; required mappings
must be tested through an actual MCP tool call.

Deploy migration **31** before starting the updated control/data plane, and update
all nodes before enabling mappings. Existing upstreams default to an empty list.
Rolling migration 31 back removes the rules, so disable mappings before reverting
to an older application version.

### Editing upstream configuration

Both MCP and REST details use transport, authentication, tool count, and recorded
health summary cards. Connection and identity settings appear first. Security and
shared headers, per-call context, and REST operations are separate expandable
sections. Header mappings are edited in a modal: **Use mappings** adds them to
the draft, **Cancel** discards modal edits, and **Save** persists the upstream.
