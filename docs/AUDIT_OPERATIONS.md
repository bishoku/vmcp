# Enterprise audit operations

## Delivery and retention

Run `go run ./cmd/audit-exporter` from `core/` as a separate service, injecting
`VMCP_AUDIT_EXPORT_DATABASE_URL`, `VMCP_AUDIT_EXPORT_CONSUMER` and
`VMCP_AUDIT_EXPORT_URL`. Use PostgreSQL `sslmode=verify-full` and HTTPS with a
trusted server certificate. Gateway egress controls apply to the destination.
Provision a restricted export identity using `deploy/postgres/audit-roles.sql`;
provision its LOGIN, password/certificate and network access out of band.
Current limitation: exporter startup also reads `tenant_runtime_identity` and
`tenants`, but the role script does not yet grant that access. Restricted logins
need narrowly reviewed identity-table access; audit INSERT defaults also read
the identity function. Correcting/testing these grants is on the production
roadmap. Do not replace restricted runtime identities with database-owner access.
The worker has its own two-connection pool, no request-path dependency and no
payload logging. One JSON database row is POSTed with its event UUID as the
`Idempotency-Key`. The sink must acknowledge durable acceptance with a 2xx and
deduplicate that UUID. A lost acknowledgement can produce a detectable repeat.

Register the consumer in `audit_export_state` **before enabling retention**.
Consumer state survives outages and deployments; disabling the service does not
remove its retention protection. A new consumer includes all retained events,
including records committed later with lower sequence IDs. Active consumers
protect events without a delivered receipt. Deactivating a consumer explicitly
waives that protection and must be an approved evidence-operator action.
Do not change a consumer's destination without managing its receipt history.

Claims expire after 30 seconds; fenced completions prevent stale workers from
marking another worker's lease complete. Retry backoff grows from 2 seconds to
one hour. After 20 failed attempts an event enters `dead`; it remains protected
from retention. Investigate destination/certificates/access, then an evidence
operator can reset that consumer/event to `state='pending',attempts=0,
next_attempt=now(),lease_until=NULL,lease_token=NULL`. Never reset delivered
receipts casually. Restrict delivery-state access: rewriting receipts can subvert
retention. Counts by state are logged each minute without event metadata.
Alert on dead deliveries, backlog growth, database errors and disk/WAL growth.

Retention keeps at most 500 candidate rows per transaction, uses skip-locked
selection and a five-second deadline, preserves operation pairs and legal holds,
and never deletes incomplete admissions. Very large operations exceeding a
batch remain retained for operator review. A legal hold on one event protects
its complete operation. The existing control-plane retention loop runs hourly;
backlogs may need scheduled bounded runs with the dedicated retention identity.
`VMCP_AUDIT_RETENTION_DAYS=0` disables deletion. The current control-plane
retention loop uses its application connection: organizations enforcing strict
separation should disable it and run retention with the dedicated role.

Indexes cover `(ts,id)` retention selection, `(operation_id,stage)` lifecycle
lookups, event UUIDs and due consumer deliveries. Evaluate `EXPLAIN (ANALYZE,
BUFFERS)` on representative volumes and record latency, buffers and WAL before
changing batching/partitioning. There is no universal production throughput or
partitioning claim; tenant workload and legal-hold distribution matter.

## Evidence and governance

Stored metadata includes verified subjects, actor IDs, issuer, client IP,
virtual server/tool/upstream names, decisions, timing, revisions and correlation
IDs. Arguments/results are excluded. This metadata can still identify people.
The privacy owner defines lawful basis, region, retention, access/deletion
procedures, litigation exceptions and notification requirements. Delete expired
browser sessions and JWT denylist entries only after their expiry; retain
incident evidence separately according to approved policy.

Assign an evidence owner for access reviews, delivery backlog, hold approval,
restore testing and incident response. Record privileged containment and receipt
changes in the organization's external change-management system. Review
application, query, export and retention identities separately. Test TLS failure
and destination authorization before onboarding each sink.

For immutable retention, deliver to an externally administered WORM/object-lock
sink and retain its acceptance receipts under a separate trust boundary. A DBA
can rewrite PostgreSQL events and receipts; the database alone is not a tamper
proof ledger. Keep externally anchored hashes/signatures where policy requires
integrity verification. This implementation does not manufacture an external
trust anchor or certify SOC 2/ISO 27001/GDPR compliance.

SOC 2 evidence owners should map admission/outcome, access control, backups,
change review and monitoring to their actual control statements. ISO 27001
owners should map access, logging, key management and incident procedures to
their risk treatment plan. GDPR owners must assess minimization, subject rights,
retention and international transfers. These are organizational acceptance gates.

Reproducible integration coverage includes sink interruption/restart, duplicate
IDs, late commits, unexported retention protection and legal-hold release. Run
with real PostgreSQL using the documented verification command. Restore evidence
must include configuration, encrypted secrets with recovery keys, audit rows,
consumer registrations, delivery receipts and legal holds before re-enabling
retention or exporters. Validate counts and replay one known pending event into
a test sink; do not send restored production evidence into an unintended sink.

Local index evidence (PostgreSQL 17, 20,000 synthetic events across 365 days):
`TestRetentionIndexPlan` selected `Index Only Scan using audit_events_retention`
for the 500-row chronological candidate scan, 504 shared buffer hits and 0.115 ms
execution on the development machine. This is a candidate-scan measurement, not
an enterprise benchmark of deletion, legal-hold joins, WAL or sink throughput.
Reproduce with `go test -race -v ./internal/audit -run TestRetentionIndexPlan`.
