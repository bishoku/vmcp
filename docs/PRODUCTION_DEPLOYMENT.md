# Deployment and release gates

## Reproducible verification

Use the Go version in `core/go.mod`, Node 22 or newer and Docker. From a clean
checkout run `npm ci --prefix web` and `cd web && npx playwright install chromium`.
Start disposable PostgreSQL 17 and Redis 7, set `VMCP_TEST_DATABASE_URL` (a role
allowed to create/drop temporary test databases) and `VMCP_TEST_REDIS_URL`, then
run `make verify`. Never point these variables at production infrastructure.
The release command sets `VMCP_REQUIRE_INTEGRATION=1` and
`VMCP_REQUIRE_SDK_INTEROP=1` and rejects skipped tests in the Go JSON results.
CI runs the same race, vet, frontend, browser and OpenAPI checks, plus dependency
and container scanning. `make images` builds planes and operational commands.

The pinned TypeScript MCP SDK is `@modelcontextprotocol/sdk` **1.32.1**. Its real
HTTP client negotiates **2025-06-18**, initializes, pings, lists/calls tools and
terminates a gateway session in `TestPinnedTypeScriptSDKInterop`. This does not
certify every SDK feature or claim support for newer protocol revisions. The
other integration tests cover bidirectional replay, resources/prompts and client
feature negotiation with gateway fixtures. Update the pin deliberately and
repeat compatibility tests. No March batching compatibility is advertised.

## Kubernetes deployment

The Helm chart is `deploy/helm/vmcp`. It expects external PostgreSQL, optional
Redis for shared rate limits, existing Secrets, immutable image references and
an operator-owned TLS ingress. It does not create weak default credentials or
install a database. Build the migration image with `SERVICE=migrate` and use it
for the pre-install/pre-upgrade Job. Provide a migration-only database owner
credential; set plane Secrets to lower-privilege runtime identities. Migrations
hold a PostgreSQL advisory lock, and the chart disables automatic plane
migrations. Startup rejects an older schema. DDL credentials need a direct
PostgreSQL connection, not PgBouncer transaction pooling.

Each plane Secret supplies `VMCP_DATABASE_URL`, `VMCP_MASTER_KEY` and its
plane-specific authentication variables. Data plane additionally needs
`VMCP_DB_LISTENER_URL` as a direct/session-pooling URL, `VMCP_PUBLIC_URL`,
`VMCP_ALLOWED_ORIGINS` and permitted upstream/IdP egress grants. Use
`sslmode=verify-full`; mount trusted CA/client certificates through your secret
operator when required. Keep encryption and cookie recovery material outside
Git. The control plane requires a strong admin bootstrap credential and/or
properly mapped OIDC roles; configure redirect URLs to the ingress origin.
Redis, when supplied, is only rate-limit state. With strict Redis failure policy,
a Redis outage makes readiness fail rather than silently relaxing quotas.

Set `controlPlane.image`, `dataPlane.image`, `migration.image` to reviewed digest
references and their `secretName` values. Configure `networkPolicy.egressCIDRs`
for PostgreSQL, Redis, upstreams, identity and approved telemetry/policy endpoints.
By default only cluster DNS egress is enabled, so an unconfigured chart cannot
reach the database. Configure `networkPolicy.ingressNamespace` for the actual
TLS ingress controller. Verify CNI enforcement and DNS behavior in your cluster.
The chart intentionally does not select an ingress vendor or expose metrics
publicly; route `/api` and `/auth` to control plane, MCP paths to data plane,
and serve the built React UI separately at the approved browser origin.

Example rendering (replace images and existing secrets for installation):

```sh
helm lint deploy/helm/vmcp \
  --set controlPlane.image=vmcp/cp:test,dataPlane.image=vmcp/dp:test,migration.image=vmcp/migrate:test \
  --set controlPlane.secretName=cp,dataPlane.secretName=dp,migration.secretName=migration
```

Pods run without root, service-account tokens, added capabilities or writable
root filesystems; `/tmp` is a bounded emptyDir. Two replicas, topology spreading,
zero-unavailable rolling updates and a one-pod disruption budget are defaults.
Set realistic resource limits after load tests. Termination grace is 105 seconds
for the gateway's drain and durable completion budgets. Configure load-balancer
SSE read timeouts above 75 seconds and disable response buffering. Reconnect and
replay are required client behaviors. No local durable audit volume is needed:
strict admission, delivery and exporter state are PostgreSQL-backed.

## Availability and disaster recovery acceptance

Use an externally operated HA PostgreSQL primary endpoint with tested promotion,
backups/PITR, synchronous durability where required, fencing and a direct LISTEN
endpoint. PostgreSQL is a required control and data-plane dependency. During
failure the gateway rejects stale authorization and unwritable readiness; it
cannot promise availability through database loss. Size connections as planes
× replicas × pool maximum plus dedicated listeners/exporter/migration capacity.

Before each production release, in a disposable cluster:

1. Run traffic and SSE clients against two replicas, roll both planes and verify
   no new admissions after drain, durable outcomes and replay from a surviving
   node. Record interrupted client operations; never automatically repeat unsafe
   calls with ambiguous upstream outcomes.
2. Promote the HA PostgreSQL standby under traffic. Confirm readiness fails
   during the write outage, both snapshots converge after promotion, LISTEN
   reconnects and authorization updates are enforced. Verify split-brain fencing
   and no acknowledged audit admission is lost under your replication settings.
3. Back up via the database operator/PITR system. Restore into an isolated
   instance, inject retained recovery keys, and check configuration revisions,
   upstream decryption, audit counts, sessions, consumer registrations, receipts
   and legal holds. Revoke restored browser sessions/agent grants as required by
   incident policy before opening access. Verify audit exporter restart against
   a test sink. Record restoration duration and achieved recovery point.
4. Test upstream timeout/SSRF, IdP/JWKS failure, Redis strict failure, unavailable
   policy/SIEM endpoints and storage exhaustion. Record memory, request latency,
   pool waits, audit backlog, WAL and disk growth. Define SLOs from measurements.

The local automated suite and Helm lint/render do not prove Kubernetes rolling
restart, HA promotion, real sink ACLs, infrastructure backups or recovery SLOs.
Those environment-dependent gates require an operator's recorded evidence before
calling a deployment production ready.

## Upgrade, rollback and support

Release CI produces Linux amd64/arm64 binaries and SHA-256 manifests as artifacts;
it does not automatically publish or deploy them. Verify checksums and image
scans before installation. Review migration SQL and run the migration Job once
before new planes. Preserve old recovery keys and follow
`SECURITY_OPERATIONS.md` for reader-first ciphertext upgrades. Down migrations
can destroy newer feature data; restoring an old binary alone is not a safe
rollback after schema/envelope changes. Test a database restore or explicitly
reviewed backward-compatible downgrade in isolation first.

No production SLA or certification is declared. See `SECURITY.md` for the current
security reporting/support policy and `AUDIT_OPERATIONS.md` for evidence owners,
restricted database roles and externally anchored immutable retention.
