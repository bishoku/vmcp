# Durable audit admission (P04)

## PostgreSQL baseline and guarantees

`VMCP_AUDIT_MODE=strict` is the data-plane default. For allowed tool calls, the gateway commits an `admission` event in PostgreSQL before opening an upstream session or dispatching the call. Admission failure returns HTTP 503 and prevents dispatch. Audit transactions explicitly set `synchronous_commit=on`; the database must itself run with durable storage, `fsync=on` and a suitable backup/failover policy. This acknowledges local PostgreSQL WAL durability, not replication quorum or immunity to storage loss.

The admission and outcome are separate facts with distinct stable event UUIDs, one operation UUID and a server-generated request UUID. `X-Request-Id` correlates the response with persisted events. The trace ID is accepted only from a syntactically valid version-00 `traceparent`; arbitrary caller request-ID text is excluded. Outcome UUIDs derive deterministically from the operation UUID so retry/recovery across recorder instances deduplicates them. Unique indexes enforce event identity and one admission/outcome per operation. Commit acknowledgement loss can cause retry, but cannot duplicate the same event UUID.

Transient connection, serialization, deadlock and availability failures retry with backoff from 25ms to 250ms, inside a five-second write budget bounded by the caller. Permanent PostgreSQL rejections, including injected disk-full/constraint errors, stop immediately. No local WAL, disk spill or broker is used. PostgreSQL-only strict mode deliberately refuses new protected operations during database failure or resource exhaustion.

Outcomes are acknowledged before a normal tool result is returned. Completion uses its own bounded context so client cancellation does not immediately erase the outcome of an admitted action. If the outcome cannot be stored, the gateway returns 503 and leaves a durable admission without an outcome. **The upstream action may already have completed. Do not blindly retry a non-idempotent tool because this response is 503.** Use the request ID and upstream records to reconcile.

The external side effect and the audit outcome cannot be atomic. A crash immediately after admission may precede dispatch; a crash after dispatch may follow a successful side effect. Both leave detectable incomplete work. Recovery surfaces the persisted admission; it never automatically replays the tool or fabricates success. P03 authorization guards still apply before dispatch and retries. A denial after admission receives a denied outcome when persistence is available.

Strict mode has no in-memory admission queue to overflow and no local disk to fill. Database pool exhaustion, database disk-full and durability acknowledgement failure block admission. The PostgreSQL fault-injection test models disk-full with SQLSTATE `53100`; it does not fill a real filesystem. Backup, replication, failover and physical disk-failure exercises remain deployment responsibilities and later operational phases.

## Administrative transactions and secret access

The control-plane `/api/v1` API always uses strict audit handling for authorized mutations, connection tests, configuration export and upstream metadata reads (which internally load encrypted connection settings). It records admission before invoking the handler. Successful configuration transactions insert `mutation` audit records in the **same transaction** as the changes. A failed mutation audit insert rolls back both changes and audit rows. Agent/admin key creation, rotation, revocation and deletion, identity binding changes, tool discovery updates and GitOps apply/prune use this path.

Each affected object contributes an identifier-only target such as `upstream:billing`, `agent-key:bot` or `identity-binding:service`; repeated writes to one object within a transaction are deduplicated. Apply can produce multiple mutation events sharing the operation ID. Mutation records include the revision visible inside that transaction. Tool records carry their authorization snapshot revision; strict standalone events and control-plane admission/outcome records with no supplied revision observe the current database revision when persisted.

Control-plane responses are held in a bounded 8 MiB buffer until outcome acknowledgement, including newly issued secrets. A response larger than this limit fails after recording the operation error; previously committed mutations still exist and must be reconciled. Rejected authentication and general read-only API requests are not covered by administrative admission; this phase does not claim comprehensive sign-in/session-security telemetry.

Actors identify authentication mode and immutable credential ID. OAuth/SSO records include verified subject and issuer; named admin tokens use their database UUID. The bootstrap token is identified as `bootstrap`, without storing its secret. The control plane records the direct socket peer address; it does not trust forwarded client-address headers. Names, slugs, client IPs, subject identifiers, correlation metadata and configured policy reasons can be identifying data and need access control and retention.

Arguments, results, request bodies, authorization headers, raw upstream errors, decrypted credentials, token secrets and URL query strings are excluded from audit rows. Invalid requested tool names are excluded rather than copied as arbitrary audit text. Identifier fields remain administrator-controlled metadata; they must not be used to encode secrets. Existing structured operational error logs are a separate surface and need the later privacy/error-sanitization work.

## Incomplete operations and diagnostics

- `GET /api/v1/audit-events?incomplete=true` lists admissions without outcomes, including currently running operations. Existing cursor pagination and filters remain available.
- Filters also support `stage`, `operationId`, `requestId`, `actorId` and status `started`.
- `GET /api/v1/audit-status` is authenticated and reports the control-plane process counters plus the durable backlog shared by both planes.
- Data-plane `GET /audit-status` reports its process counters and the same durable incomplete count/oldest age; it returns 503 if the database query fails. This is a probe endpoint: restrict its network access like other operational probes.
- Data-plane `/readyz` retains its authorization freshness fields and adds `audit` process counters. It does not perform an additional audit database probe.

Process counters expose `blockedAdmissions`, `failedWrites`, `deadLetters`, `dropped`, `pending` and `oldestPendingAgeSeconds`. They reset on process restart. `deadLetters` counts permanent write rejections/affected failed-batch entries; it is **not** a durable dead-letter queue. Durable admissions without outcomes are the recoverable evidence for admitted strict operations. Incomplete counts survive process restart. Oldest pending age in best-effort mode is a conservative age from the current backlog episode, not a per-entry histogram.

Investigate admissions older than expected execution plus outcome-write budgets. Correlate with upstream records, then record a known reconciled outcome using the operation ID through a controlled administrative procedure. There is no automatic reconciliation API in this phase. Treat unknown execution as unknown, even when a client disconnected or an error response was returned. Successful audit outcome records establish gateway operation completion, not client receipt of the response.

## Best-effort compatibility mode

`VMCP_AUDIT_MODE=best_effort` retains an explicitly weaker data-plane option. It queues up to 4096 events and holds one batch of up to 200. A failed batch is retained with the same UUIDs and retried; the flusher stops draining the bounded queue until persistence recovers. Full queues drop newly offered events and increment `dropped`. Permanent rejection blocks that batch and contributes to `deadLetters`; it does not silently discard the batch and continue.

Cancellation stops new submissions and gives final batches a total ten-second flush budget. If PostgreSQL remains unavailable, remaining best-effort events are counted as dropped and an explicit shutdown-incomplete log is emitted. Process crash loses queued best-effort events. This mode neither acknowledges admission before tool dispatch nor provides complete audit coverage. It must not be presented as the strict enterprise guarantee.

## Retention, export and rollout

Retention preserves an admission and its related events while it has no outcome. Once an outcome exists, the operation's events become eligible together only after the outcome and individual rows are older than the retention cutoff. This makes incomplete work visible rather than silently expiring it. Unresolved admissions can accumulate indefinitely; operators need a reconciliation and privacy policy.

The separate `audit-exporter` delivers HTTPS JSON with durable consumer receipts,
retries and recoverable dead letters. Retention protects incomplete operations,
legal holds and events without delivered receipts for active consumers. Delivery
is at least once; the sink must deduplicate stable event IDs. PostgreSQL alone is
not a tamper-evident archive. See [audit operations](AUDIT_OPERATIONS.md) for sink,
retention and least-privilege limitations. No compliance certification is implied.

Migration `00018_durable_audit.sql` adds event identity, lifecycle metadata, correlation/indexes and independent checkpoint storage. Existing rows receive event UUIDs and `stage='event'`; historical ownership/correlation cannot be reconstructed. Apply schema 18 through the control plane before replacing data-plane binaries. For an enterprise strict guarantee, drain every old writer and replace it before restoring traffic. Rolling mixed versions can still write legacy best-effort records.

Rollback removes new correlation/lifecycle fields and deletes `started` rows to restore the old status constraint. **Archive the complete audit tables and exporter checkpoints before rollback**; otherwise incomplete-operation evidence is lost. Rollback requires a coordinated maintenance window, not a transparent production downgrade.

## Regression evidence

- `core/internal/audit/durable_test.go`: admission acknowledgement, bounded retries, permanent rejection/database failure, stable outcome UUIDs, trace validation, retained failed batches, full queues and explicit shutdown loss accounting.
- `core/internal/audit/persistence_test.go`: real child-process abrupt exit after admission, restart inspection, outcome replay deduplication, and retention preserving incomplete operations and operation pairs.
- `core/internal/virtual/durable_audit_test.go`: strict success lifecycle, database outage and injected PostgreSQL disk-full block upstream initialization; failed outcome leaves an incomplete admission after dispatch; arguments never enter audit rows.
- `core/internal/controlplane/audit_test.go`: administrative admissions/outcomes, concrete mutation targets, transactional rollback on audit failure, blocked admission, privileged export, named token attribution, query validation and secret/payload exclusion.
- `core/internal/store/migrations_test.go`: schema-17 upgrade and rollback preserving historical audit events, alongside earlier migration tests.

The current implementation also includes shutdown/resource bounds, metrics and HTTPS
SIEM delivery. Real full-disk/restore drills, scalable retention scheduling and
restricted-role deployment verification remain production acceptance work.
