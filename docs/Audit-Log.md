# Audit log

The data plane defaults to `VMCP_AUDIT_MODE=strict`: protected tool/resource/prompt calls receive a durable PostgreSQL admission before dispatch and a separate outcome. A failed admission prevents dispatch. A missing outcome identifies in-flight or incomplete work; an upstream action may have completed even when outcome persistence fails and the client receives 503.

By default, administrative mutations and their identifier-only audit records commit in the same database transaction. Privileged export and upstream metadata reads are also admitted and audited. Payload capture is disabled by default; credentials remain excluded by mandatory filters. See the optional capture policy below.

| Field | Meaning |
|---|---|
| `eventId`, `operationId`, `requestId`, `traceId` | Stable event identity and correlation; responses carry `X-Request-Id` |
| `stage`, `schemaVersion` | `event`, `admission`, `mutation` or `outcome`, and metadata schema version |
| `actorType`, `actorId`, `subject`, `issuer` | Credential identity and verified OAuth/SSO identity where applicable |
| `action`, `target`, `configRevision` | Operation, identifier-only target and observed/transaction revision |
| `time`, `virtualServer`, `method` | When, where and which method |
| `agentKey`, `clientIp` | Principal label and caller address |
| `tool`, `upstream` | Validated tool and upstream identifiers |
| `status` | `started`, `ok`, `tool_error`, `error` or `denied` |
| `reason`, `durationMs`, `redactions` | Fixed/configured reason, duration and masking count |

The Audit page's **Activity** filter separates **Virtual MCP calls**, **Control
plane API**, **System activity**, and **Upstream calls only**. The default is
**All activity**. Virtual MCP includes tool/resource/prompt call events plus
upstream attempts explicitly linked to a stored virtual parent operation. Control
plane API includes reads, writes and their linked upstream attempts. System
activity contains meaningful automatic catalog changes and historical maintenance
records. New routine discovery and subscription maintenance are operational logs
and metrics, not audit events.
Upstream calls only selects events recorded with upstream scope; system-scoped
upstream traffic stays under System activity.

The API accepts `activity=virtual|control|system|upstream`. Filtering happens in
PostgreSQL before cursor pagination and applies equally to Operations and Raw
events. Other filters still intersect it. Filter links preserve the selection.
The call's `virtualServer`, shared trace/request IDs or method name alone do not
establish its origin. Legacy/unclassified records remain visible under All
activity; an upstream record whose parent has been removed cannot be attributed
to a virtual/control operation by this filter.

`GET /api/v1/audit-events` supports filters and cursor pagination. Use `incomplete=true` for admissions without outcomes; this includes currently running calls. The Audit UI exposes this filter. `GET /api/v1/audit-status` reports durability counters and the persisted incomplete backlog.

The **Operations** view groups a tool call and its explicitly linked upstream
operations using `parentOperationId`. The parent tool outcome supplies the row's
status and duration; upstream attempts keep their own operation IDs and remain
individually inspectable in Details and **Raw events**. Shared request/trace IDs
alone never merge operations. Grouping applies to loaded records: filters or
pagination can omit the parent/outcome. A child outcome does not prove that the
parent completed; load more records or open the parent operation to investigate.

`VMCP_AUDIT_RETENTION_DAYS` defaults to 90; `0` keeps everything. Incomplete operations are preserved until reconciliation; completed operation pairs are retained through the outcome cutoff. Explicit `best_effort` data-plane mode can lose queued events on overflow, prolonged shutdown failure or crash and does not provide complete coverage.

See [Audit durability](AUDIT_DURABILITY.md) for exact acknowledgement guarantees, outage and shutdown behavior, reconciliation, migration/rollback requirements and SIEM limitations. See [audit operations](AUDIT_OPERATIONS.md) for the separate
independent HTTPS/Kafka/RabbitMQ exporters, stable event IDs, legal holds and consumer-aware retention. Some
early catalog validation/denial paths still lack equivalent decision records;
complete denial coverage is on the [production roadmap](../maintainer/Production-Roadmap.md).

Configure destinations with [independent audit exporters](AUDIT_EXPORTERS.md).

## Background work and meaningful system changes

Automatic discovery does not create audit admissions/outcomes for each poll,
retry, connection or list request. Unchanged results update refresh freshness
without adding audit events or changing the configuration revision. Routine
resource-subscription activation, reconnects, lease renewal and worker shutdown
also do not create system audit lifecycles. The client's original subscribe /
unsubscribe requests retain their normal virtual-call auditing. Administrative
configuration/permission changes retain their existing audit behavior.

A discovered tool/resource/template/prompt addition, modification or removal,
capability change, or instructions change creates one `upstream.catalog_changed`
mutation event. Its upstream, configuration revision and bounded change summary
identify what changed. The summary includes sorted object identifiers, not
schemas, descriptions or instruction content; changes exceeding 8 KiB are
summarized by exact counts. Payload capture is not used for this metadata event.
Strict mode commits the catalog change and audit fact atomically: an audit write
failure rolls back the change for retry. Relax mode queues the fact after commit
and retains its documented loss guarantees. Failed probes produce no catalog
mutation event and retain their normal retry behavior.

Operational failures use warning logs; routine completions and subscription
activation use debug logs. Logs carry configured server identifiers and fixed
outcome/reason categories, without session IDs, resource URIs, payloads or raw
upstream errors. Both planes expose `vmcp_background_operations_total` with fixed
`activity` (`discovery`, `subscription`) and `outcome` labels. Discovery reports
`changed`, `unchanged`, `error` or `cancelled`; subscription workers report their
final `ok`, `error` or `cancelled` outcome when they finish. This counter is local
to each process and resets on restart.

System payload-capture settings do not re-enable routine maintenance auditing.
Existing audit history is retained; no records are deleted by this change.

## Payload capture and masking

Migration 29 adds optional, encrypted audit payloads in `audit_payloads`, separate
from immutable metadata. Capture defaults to off. In **Settings → Audit capture &
masking**, administrators select `strict` or `relax`, request/response capture for
virtual calls, upstream calls, management APIs, the system payload scope and API
reads, a payload size limit, payload retention, global RE2 rules and exporter
consumer permissions. An empty mode preserves the deployment default: strict for
the control plane; `VMCP_AUDIT_MODE` for the data plane (`best_effort` remains an
alias for relax there). Historical rows are marked `legacy`, not retrospectively
claimed as strict.

Both modes write metadata and any required payload in one PostgreSQL transaction.
Strict acknowledges that transaction before dispatch and before returning a
normal outcome. Relax prepares masked, encrypted content and offers the entire
event to a bounded background queue, without waiting for database storage. Its
queue is bounded by both count and 32 MiB of accounted memory; overflow, process
crash and failed shutdown can lose entire events. A request may have an admission
without an outcome, or an outcome without a persisted admission in relax mode.
Neither absence proves execution failure. Preparation still consumes CPU in
relax mode; it intentionally avoids queuing raw, unmasked payloads.

The operation retains its starting mode and policy revision. Saving settings
never changes an operation halfway through. Configuration revision notifications
propagate settings to data-plane snapshots; `/audit-status` reports the applied
policy revision and mode. Control-plane mutations remain transactionally audited
in strict mode. In relax mode their mutation events are queued only after commit,
so audit failure cannot roll back the configuration change. A crash between the
configuration commit and enqueue is an accepted loss window.

Payloads represent the selected boundary: virtual requests contain MCP params,
virtual responses contain results/errors, MCP upstream records contain actual
protocol params/results, and REST upstream records contain the constructed method,
path, query and body plus the bounded service response with its HTTP status. Enabling upstream capture
adds separate admission/outcome records for protocol calls, including initialization
and caller-level retries, linked through `parentOperationId` and `requestId`.
Credential acquisition traffic and authorization headers are excluded. Transport
internal OAuth refresh/replay remains inside its enclosing protocol exchange.
Progress/logging notifications keep metadata evidence without accumulating stream
payloads. Static OpenAPI downloads are excluded from payload capture.

Audit masking never changes the messages forwarded to agents/upstreams. Mandatory
secret-field filters are followed by global and applicable virtual-server/upstream
rules. Local rules add protection; they cannot disable inherited global rules.
JSON objects and arrays encoded inside strings (including MCP text results) also
receive secret-field filtering within the same traversal budgets. Their outer
string type is preserved. If a regex matches the original encoded JSON string,
the entire string is redacted so decoding cannot defeat matches across JSON syntax.
Regex matches on a value are merged before replacement. JSON structure is
preserved; sensitive numeric values become redaction strings. Binary content is
omitted or replaced with a marker. Invalid, oversized and policy-excluded documents
have explicit states rather than unmasked fallbacks. The default stored-content
budget is 64 KiB, configurable up to 1 MiB. Input traversal also has independent
size, depth, node and match-count bounds. Rules cannot guarantee detection of
arbitrary secrets or personal information.

Local rules are managed on the virtual server's Protection screen and on the
saved upstream screen. Global and local panels share a request/response preview
helper backed by the production masking engine. Samples stay in component memory
and are excluded from audit payload capture. Local previews merge the edited
local rules with the current saved global/context policies and reject stale
revisions. Samples are interpreted at the selected boundary; the helper does not
call an upstream or predict its response. Saving settings uses optimistic
concurrency and rejects stale writes with HTTP 409. `auditSettings` is included in
configuration export/apply; applying it requires administrator access.

Only administrators can read payloads, edit policies or use previews. Payload
reads are themselves audited, without recursively capturing their returned
content. Metadata lists do not load/decrypt payloads. Audit details fetch content
on demand and show when content has expired. Local policies are keyed by slug;
configuration export/apply can also manage policies for objects not yet created.

Payload retention defaults to seven days, independently of the metadata retention
environment setting. Metadata is retained until its payload has been released by
payload cleanup, even when metadata retention is shorter. Cleanup runs even when metadata retention is disabled. Legal
holds protect related payloads across the correlated request. Active consumers
configured to receive payloads retain pending content until a delivery snapshot
exists. Snapshots containing payloads are encrypted, frozen before first send and
reused byte-for-byte on retry; snapshots are cleared after acknowledged delivery.
Revoking payload export blocks pending payload snapshots rather than sending their
contents or changing their body under the same event ID.
Pending/dead deliveries can therefore retain encrypted content beyond its normal
expiry. Retention and external destination lifetimes must be operated together.

Exporters emit one combined message per audit event, not separate payload messages
or one collapsed message for an entire operation. Payload export is opt-in by
consumer name in Settings. Such exporters need `VMCP_MASTER_KEY` and the applicable
keyring/tenant configuration; metadata-only exporters can continue without a key.
Run the updated `deploy/postgres/audit-roles.sql` grants for the additional tables.
The secret rotation command includes payloads and pending export snapshots.

Apply migration 29 and role grants before deploying the new binaries. Keep capture
disabled until every writer/exporter is updated; older writers do not implement
these policies. Rollback drops payloads, settings and frozen envelopes: archive
and coordinate a maintenance window before downgrading.

The audit detail view opens in a wide modal (full screen on mobile). Select a
logical operation step to inspect its request and response side by side. The
summary uses the root outcome and its duration, not the sum of child durations.
Explicit parent IDs identify child relationships; incomplete selections remain
marked as incomplete. Technical context and every raw record are available in
an expandable section. Payloads are loaded on demand by administrators, with
capture-disabled, redacted, expired, and retrieval-error states kept distinct.
