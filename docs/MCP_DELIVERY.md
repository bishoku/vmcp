# MCP delivery contract

The data-plane supports the 2025-06-18 contract. Initialized tools/call requests
receive POST SSE; GET SSE carries tools/list_changed or resumes the original
POST stream using Last-Event-ID. Other requests retain JSON responses. Clients
must send the session and protocol headers on reconnect and client replies.
Implementation: core/internal/virtual/handler.go and streams.go.

## Ownership, recovery and cancellation

Migration 19 stores operation fences, exclusive stream leases, encrypted replay
events and pending server requests. HTTP disconnect does not cancel a tool.
A client can reconnect to another replica and reply there to an upstream ping.
Client request IDs, physical upstream IDs, progress tokens and server-request
IDs are mapped separately. Explicit cancellation reaches the executing worker
through shared operation state. Active workers renew every second; leases last
15 seconds. Expired workers cannot publish or renew and are marked abandoned.
Work is never automatically reexecuted after worker loss. Clients must reconcile
ambiguous outcomes with audit evidence before retrying side-effecting tools.

PostgreSQL is the durable queue. A second dedicated LISTEN connection per DP
wakes readers; polling every 250ms preserves delivery if notifications are lost.
Use VMCP_DB_LISTENER_URL with direct/session pooling for both listeners.
Replay event IDs are opaque UUIDs and are scoped to the authenticated session
and original stream. Stream ownership excludes concurrent live delivery;
replay intentionally redelivers events after the supplied cursor.

Cancellation is best effort at the physical server: vMCP sends
notifications/cancelled with the physical request ID, but cannot undo an
upstream side effect or guarantee that an upstream honors cancellation.

## Bounds and security

Shared active operations are limited to 1024 globally, 128 per virtual server,
and 16 per principal. Each operation allows 32 pending server requests and
256 events, reserving one terminal event; intermediate ciphertext totals are
limited to 2MiB and all events to 12MiB per operation. Upstream frames remain
limited to 10MiB. Capacity exhaustion fails closed. GET streams also use HTTP
admission budgets. Wake channels contain one coalesced signal.

Replay expires after 15 minutes; cleanup removes at most 100 streams and 100
terminal operations per tick. Capacity bounds are safety limits, not a measured
throughput guarantee. Monitor PostgreSQL storage, WAL and latency under load.
Payloads use the configured AES-GCM box; use the [keyring and rotation procedure](SECURITY_OPERATIONS.md#encryption-upgrades-and-recovery)
when changing encryption keys. Event payloads do not appear in audit metadata.

Reconnects reauthenticate; live streams check identity, snapshot freshness and
authoritative session state each second. Session termination closes a healthy
stream within about one second plus query scheduling. Key/policy revocation
also depends on the existing snapshot freshness bound (30 seconds).
Progress and client responses pass through PII masking; durable metadata audit
writes precede forwarding. Unsupported client features return MethodNotFound;
sampling, roots and elicitation require explicit permissions and client negotiation;
see [Client features](MCP_CLIENT_FEATURES.md).

## Discovery and shutdown

Tool-list notifications seen on active physical POST/legacy SSE connections
enqueue reconciliation. Two control-plane workers use shared SQL leases and
audited discovery; enabled catalogs also refresh on a 30-second polling cycle.
Configuration revision changes produce deduplicated downstream list-change
events. The gateway does not open idle upstream GET streams, so unsolicited physical
changes while idle rely on polling. This latency must not be described as
instant upstream push synchronization.

GET streams reconnect after 65 seconds; POST streams allow 75 seconds and
workers allow 70 seconds including initialization and audit work. Shutdown
stops admission, drains HTTP and workers, then cancels workers with an additional
10-second completion allowance. Compose grants 105 seconds. Retained events
survive replica restart; terminating a worker can still leave an ambiguous
physical side effect and incomplete durable audit admission.

Regression coverage: core/internal/virtual/streams_test.go exercises two-node
disconnect/replay/replies, mapped cancellation and session termination;
core/internal/delivery/store_test.go checks fencing, encrypted replay, isolation,
ownership, notification deduplication, queue pressure and expiry;
core/internal/store/migrations_test.go checks upgrade/rollback preservation.
