# Production deployment

## Before you deploy

Choose a version and follow [Release installation](Release-Installation.md) for
packaged charts and digest-pinned images. Validate the target environment with
the availability and recovery procedures below. Source-build and release
verification instructions are in the [maintainer guide](../maintainer/Development.md).

## Kubernetes deployment

The [Helm chart](../deploy/helm/vmcp/README.md) deploys the UI, control/data planes,
optional independent audit exporters, and a migration Job. External PostgreSQL and
Redis are the default; pinned PostgreSQL, Redis, Kafka and RabbitMQ subcharts are
optional. Production images require immutable digests, explicit credentials, TLS,
and approved dependency network destinations.

Start with [external-service values](../deploy/helm/vmcp/examples/production.yaml),
then optionally overlay [audit exporters](../deploy/helm/vmcp/examples/exporters.yaml)
and [monitoring](../deploy/helm/vmcp/examples/monitoring.yaml). All examples contain
placeholders to replace. Each workload has resources, scheduling, security settings,
PDBs and optional HPA. Configure runtime settings and Secret sources through values;
existing Secrets support your chosen secret manager. The web image accepts runtime
gateway configuration and a release-specific control-plane proxy endpoint.

Fresh installs run migration after normal resources can exist. Upgrades use a
blocking pre-upgrade hook with already provisioned migration credentials and
network access; stage changes to those prerequisites first. Manual migration mode
supports external pipelines. Helm rollback does not reverse database migrations.
Keep migration-owner credentials separate from runtime and exporter LOGIN roles.
See the chart guide for bundled dependency image availability, role provisioning,
TLS mounts, LISTEN pooling constraints, ingress streaming settings and recovery.

```sh
helm repo add bitnami https://charts.bitnami.com/bitnami --force-update
helm dependency build deploy/helm/vmcp
helm lint deploy/helm/vmcp -f my-production-values.yaml
helm upgrade --install vmcp deploy/helm/vmcp --namespace vmcp --create-namespace \
  -f my-production-values.yaml --wait --wait-for-jobs --timeout 15m
```

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

Published releases provide packaged Helm charts, image inventories and checksums;
standalone Linux binaries are retained separately as Actions artifacts. See
[Release installation](Release-Installation.md). Publishing a release does not deploy it.
Verify checksums and image references before installation. Review migration SQL and run the migration Job once
before new planes. Preserve old recovery keys and follow
[Security operations](SECURITY_OPERATIONS.md) for reader-first ciphertext upgrades. Down migrations
can destroy newer feature data; restoring an old binary alone is not a safe
rollback after schema/envelope changes. Test a database restore or explicitly
reviewed backward-compatible downgrade in isolation first.

No production SLA or certification is declared. See [security reporting policy](../SECURITY.md) for the current
security reporting/support policy and [Audit operations](AUDIT_OPERATIONS.md) for evidence owners,
restricted database roles and externally anchored immutable retention.
