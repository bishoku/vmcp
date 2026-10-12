# Audit log

The data plane defaults to `VMCP_AUDIT_MODE=strict`: protected tool/resource/prompt calls receive a durable PostgreSQL admission before dispatch and a separate outcome. A failed admission prevents dispatch. A missing outcome identifies in-flight or incomplete work; an upstream action may have completed even when outcome persistence fails and the client receives 503.

Administrative mutations and their identifier-only audit records commit in the same database transaction. Privileged export and upstream metadata reads are also admitted and audited. Arguments, results and credentials are excluded.

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

`GET /api/v1/audit-events` supports filters and cursor pagination. Use `incomplete=true` for admissions without outcomes; this includes currently running calls. The Audit UI exposes this filter. `GET /api/v1/audit-status` reports durability counters and the persisted incomplete backlog.

`VMCP_AUDIT_RETENTION_DAYS` defaults to 90; `0` keeps everything. Incomplete operations are preserved until reconciliation; completed operation pairs are retained through the outcome cutoff. Explicit `best_effort` data-plane mode can lose queued events on overflow, prolonged shutdown failure or crash and does not provide complete coverage.

See [Audit durability](AUDIT_DURABILITY.md) for exact acknowledgement guarantees, outage and shutdown behavior, reconciliation, migration/rollback requirements and SIEM limitations. See [audit operations](AUDIT_OPERATIONS.md) for the separate
independent HTTPS/Kafka/RabbitMQ exporters, stable event IDs, legal holds and consumer-aware retention. Some
early catalog validation/denial paths still lack equivalent decision records;
complete denial coverage is on the [production roadmap](../README.md#roadmap-to-production-readiness).

Configure destinations with [independent audit exporters](AUDIT_EXPORTERS.md).
