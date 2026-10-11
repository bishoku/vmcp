# Resources and prompts (P09)

The 2025-06-18 gateway exposes explicitly selected resources, URI templates and
prompts. Tool selection grants no resource or prompt access. The implementation
is in `core/internal/virtual/catalog.go`, `config/catalog.go`, `store/catalog.go`
and migration 20. Protocol reference:
[resources](https://modelcontextprotocol.io/specification/2025-06-18/server/resources),
[prompts](https://modelcontextprotocol.io/specification/2025-06-18/server/prompts),
[completion](https://modelcontextprotocol.io/specification/2025-06-18/server/utilities/completion).

## Management and discovery

Refreshing an upstream also discovers advertised catalogs. Each family is
bounded to 100 pages, 10,000 objects and 10MiB of catalog JSON; duplicate
identities and repeated cursors fail discovery. Metadata and extension fields
are retained. `GET /api/v1/upstreams/{slug}/catalog` reads the bounded cached
catalog. The Virtual Servers editor selects resources, templates and prompts,
with aliases for resources/prompts and description overrides. Removed objects
remain visible as orphaned selections and are excluded from MCP discovery.
Description overrides retain a basis for drift reporting.

Virtual-server create/PUT/PATCH and export/apply include a `catalog` array:

```json
{"catalog":[
  {"upstream":"docs","kind":"resource","name":"doc://readme"},
  {"upstream":"docs","kind":"template","name":"doc://items/{id}"},
  {"upstream":"assistant","kind":"prompt","name":"review","alias":"approved_review"}
]}
```

ETags protect concurrent edits; GitOps preview/apply remains transactional and
preserves existing orphan selections. Unknown new selections are rejected.
Deleting an upstream selected by any feature family requires force=true.

## Routing, privacy and capabilities

Resources use `vmcp://{upstream}/resource/{base64url(originalURI)}` or an explicit
resource alias. Prompts use an alias or `{upstream}__{name}`. Templates retain
RFC6570 expressions under `vmcp://{upstream}/template/`; expanded URIs must match
the selected physical template. Matching uses uritemplate/v3 v3.0.2. These URIs
are identifiers routed through MCP requests, never gateway HTTP fetch URLs.
Resource read results must refer to the requested physical URI and are rewritten
to the exposed URI. Unselected names and direct physical URIs fail closed.

Supported operations: resources/list, resources/templates/list, resources/read,
resources/subscribe, resources/unsubscribe, prompts/list, prompts/get and
completion/complete. Lists page at 200 objects with revision-bound cursors.
Prompt arguments must be strings with known names and required arguments present.
Completion references route only through selected prompts/templates and upstreams
that advertise completion. Resource/prompt capabilities are omitted when no
objects in that family are selected. Subscription support is advertised only
when selected objects originate from a subscription-capable upstream and durable
MCP delivery is enabled.

Calls share existing principal/server quotas, durable audit admission/outcome,
freshness checks and MCP SSE delivery. Content-bearing arguments can be masked
without changing routing fields. Results and metadata are masked when PII rules
are enabled; opaque routing identities remain stable. Binary blobs, images and
audio fail closed under an active PII policy. Regex masking does not inspect
binary encodings or guarantee prompt-injection prevention. Resource and prompt
errors use static messages; payloads are excluded from audit and operational logs.

## Subscription lifecycle and limits

`virtual/subscriptions.go` persists grants in PostgreSQL and leases physical
watchers to any available replica. Each watcher initializes a separate physical
session, subscribes, and receives unsolicited changes over Streamable HTTP GET
or legacy SSE. Only the selected URI is forwarded. Encrypted hints are appended
atomically with metadata audit evidence to the session's durable GET stream;
Last-Event-ID and session ownership rules from P08 apply. A hint can be queued
before the client opens GET. PostgreSQL fencing rejects superseded workers.

Limits are 32 subscriptions cluster-wide and 16 per session. Each replica runs
32 bounded workers. Physical leases last 15 seconds and renew every second,
including during handshake. Workers rotate after 65 seconds; interrupted streams
are reconnected. A crashed replica is replaced after lease expiry; update hints
may be missed during physical reconnect, so clients should refresh subscribed
resources after reconnect. Hints are not an exactly-once change feed. Replay is
bounded by P08's stream limits and 15-minute retention.

Subscription grants expire after 15 minutes; clients renew by subscribing again.
Unsubscribe deletes the shared grant and stops its physical watcher within about
one second; physical unsubscribe/session close is best effort. Session termination,
selection removal and expired/revoked agent keys stop watchers. Policy/key changes
also depend on snapshot freshness (default 30 seconds). JWT-backed watchers retain
the accepted subscription grant until its expiry or binding/session revocation;
every client GET/reply still revalidates its current JWT. Arbitrary provider JWT
early revocation is not supplied by P09.

Catalog changes observed during active calls enqueue control-plane discovery;
the existing 30-second catalog polling fallback covers idle upstreams. Revision
changes generate deduplicated resource/prompt list notifications on GET streams.
A complete platform still needs the P10 client-feature and P11 privacy/identity
controls; P09 does not make the application production ready by itself.

Regression coverage includes `internal/e2e/catalog_test.go` (real PostgreSQL,
two replicas, namespace isolation, template routing, PII/binary policy and
subscription recovery), `controlplane/catalog_test.go` (discovery, ETags,
GitOps and orphan/drift handling), `virtual/catalog_test.go` (adversarial routing,
arguments and pagination), `upstream/catalog_test.go` (malformed discovery),
`store/migrations_test.go` and `web/src/pages/CatalogPicker.test.tsx`.
