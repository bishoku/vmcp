# vMCP Helm chart

The chart deploys HTTP MCP API planes, the management UI, optional independent
audit exporters, and an optional dedicated-tenant router. It supports external
infrastructure and pinned optional PostgreSQL, Redis, Kafka and RabbitMQ subcharts.
Kubernetes 1.27+ and Helm 3.17+ are required. CI exercises Kubernetes 1.32.

## Installation with external infrastructure

Use `examples/production.yaml` as a starting point. Its domains, addresses and
image digests are placeholders, not runnable production credentials.

1. Publish separate images built with `core/Dockerfile` and `SERVICE=control-plane`,
   `data-plane`, `migrate`, and any enabled exporter. Build `web/Dockerfile` for the UI.
   Configure complete image references including `@sha256:...`. Supply registry
   access through `imagePullSecrets`. Production digest enforcement is on by default.
2. Provision PostgreSQL and Redis, database LOGIN roles and Secrets described below.
3. Set real public hosts, ingress TLS, network destinations and trusted proxy CIDRs.
4. Restore the locked dependencies and install:

```sh
helm dependency build deploy/helm/vmcp
helm lint deploy/helm/vmcp -f my-production-values.yaml
helm upgrade --install vmcp deploy/helm/vmcp \
  --namespace vmcp --create-namespace \
  -f my-production-values.yaml \
  --wait --wait-for-jobs --timeout 15m
```

The defaults do not contain application images, encryption keys or database
credentials. Missing required values fail rendering. Keep production credentials
out of Git and shell command arguments.

## Configuration and Secrets

`values.yaml` is the configuration reference; `values.schema.json` validates
supported shapes. `runtime` provides structured pool, audit, OIDC, agent JWT,
egress and public-endpoint settings. `controlPlane`, `dataPlane`, `web`,
`exporters.http`, `exporters.kafka`, and `exporters.rabbitmq` have independent
workload configuration. `workloadDefaults` supplies shared defaults.

Secret sources support either an existing Secret or a chart-created value:

```yaml
credentials:
  masterKey:
    existingSecret: vmcp-encryption
    key: master-key
  adminToken:
    existingSecret: vmcp-admin
    key: token
database:
  controlPlane:
    existingSecret: vmcp-database
    key: control-plane-url
```

Alternatively set `existingSecret: ""` and `value: ...` in a private values file.
The chart creates a deterministically named Secret. It never generates or rotates
credentials on upgrade. Master keys must decode to 32 bytes. Existing Secret
contents are validated by the application, not read during chart rendering.
Inline Secret values may use Helm templates, for example
`postgres://user:password@{{ .Release.Name }}-postgresql:5432/vmcp?sslmode=verify-full`.
Use URL-encoded credentials in connection URLs. Template expansion is for trusted
operator-owned values only.

Full database URLs preserve pgx TLS/connection options and avoid reconstructing
credentials from Kubernetes Secret contents. Separate `database.controlPlane`,
`database.dataPlane`, `database.migration`, optional `database.listener`, and each
exporter's `database` source support independent roles. Provision and review SQL
grants before startup; the chart does not assume one account can safely serve all
roles. Use the audit role examples in `deploy/postgres/audit-roles.sql` after
migration. Control-plane retention currently runs in the control-plane process,
so its database privileges must account for the configured retention policy.

Use `sslmode=verify-full` and mount PostgreSQL CA/client certificates when needed.
Set `database.listener` to a direct/session-pooled PostgreSQL endpoint when normal
traffic uses transaction pooling. Migration credentials need DDL ownership and
a direct/session-pooled endpoint for the advisory lock. Budget all pools,
listeners, exporter connections, migrations, monitoring and administrative reserve.

`rateLimit.enabled` enables shared Redis counters and requires a `redisURL` source.
The default failure policy is `strict`; `rediss://` URLs configure TLS. Enabling
the Redis subchart and enabling shared quotas are separate choices.

Each component also accepts:

- `env` for non-secret application environment variables, including OTel settings.
- `secretEnv` mapping variable names to the same Secret source structure.
- `envFrom` for operator-managed bulk environment sources.
- `extraVolumes` and `extraVolumeMounts` for CA files, mTLS keys and other settings.

Explicit chart-managed variables cannot be overridden through `env`/`secretEnv`.
Use structured runtime settings for the variables they manage. Existing Secrets from External
Secrets Operator, Sealed Secrets, or another operator work without installing that
operator as part of vMCP. Inline Secret changes trigger a rollout; existing Secret
changes need your reloader or an explicit `kubectl rollout restart`.
`extraSecrets` can create operator-named Secrets with `{name, data}` entries,
including multi-key credentials needed by optional subcharts. Names must be unique
within the namespace. Helm stores inline values and Secrets in release history;
Kubernetes Secrets are not inherently encrypted at rest. Use cluster encryption,
RBAC and an external secret manager where required.

## Workloads and web runtime

Planes and UI default to two replicas, zero-unavailable rolling updates, PDBs,
CPU/memory requests and limits, and preferred node spreading. Configure stricter
zone/node placement with `affinity` or `topologySpreadConstraints` when cluster
capacity permits it. Every component supports resources, scheduling, annotations,
service-account identity, bounded temporary storage, probes and optional HPA.
HPA requires the metrics API and CPU requests. Size database connections against
HPA maximums. Prefer concurrency/load-derived scaling for long-lived MCP streams;
CPU alone may not represent demand.

Containers run without root, privilege escalation, added capabilities or service
account tokens, with read-only root filesystems and RuntimeDefault seccomp. The
105-second default termination grace must be at least 15 seconds longer than
`runtime.drainTimeout`. Startup/readiness checks gate API service traffic on the
schema and actual dependency health. Dependency outages should not trigger
liveness restart loops. Exporters have no HTTP health endpoint: the chart does
not attach misleading HTTP probes to them. Monitor their logs and durable receipt
state; Pod readiness alone does not prove successful SIEM delivery.

The web image listens on unprivileged port 8080. `VMCP_CONTROL_PLANE_URL` configures
its nginx proxy at startup; Helm supplies the release-specific Service address.
The UI loads public gateway configuration from `/runtime-config.js`, provided by a
ConfigMap with `Cache-Control: no-store`. One web image works across environments.
This file must contain only public settings. The existing Vite build-time option
remains a fallback for development.

## Migration and upgrade lifecycle

On first installation, migration is an ordinary revision-named Job, so its
Secrets, ServiceAccount and optional PostgreSQL can be created by the same
release. Planes reject an old schema and may restart until migration completes.
Use `--wait --wait-for-jobs` and a timeout covering dependency startup and retries.
The Job has a 10-minute deadline, bounded retries and one-day cleanup TTL.

On upgrade, migration is a blocking `pre-upgrade` hook, before application rollout.
It uses the **already provisioned** migration Secrets, ServiceAccount, certificate
volumes, network policies and database endpoint. Stage changes to those resources
before an upgrade that needs them, or use externally managed Secrets/resources.
An upgrade cannot bootstrap a newly added database dependency before its hook;
provision it separately first. A newly enabled migration ServiceAccount likewise
must exist before the hook runs. This is an explicit Helm lifecycle constraint.

Set `migration.mode: manual` to omit the Job and run migrations in your deployment
pipeline before updating workloads. This supports GitOps controllers whose hook
semantics differ from Helm CLI. `migration.enabled: false` also omits its Secrets
and ServiceAccount. Plane startup schema validation always remains enabled.

Migrations serialize with an advisory lock. A failed upgrade must be investigated;
Helm `--atomic` rollback does not reverse committed DDL. Review compatibility with
old pods before every schema change, back up first, and use additive/expand-contract
migrations for rolling upgrades. Retain keys and tenant identity across restore.
Do not change `tenantId` against an existing database.

## Ingress and network isolation

The optional Ingress serves the console host through web/nginx (`/api` and `/auth`
proxy to control plane), and the gateway host only under `/mcp/` and
`/.well-known/oauth-protected-resource`. It does not expose `/metrics`, `/readyz`,
`/healthz` or `/audit-status` publicly. Configure your ingress class, TLS Secret,
SSE read/send timeouts and disabled response buffering. The supplied example uses
nginx annotations; use equivalent settings for your chosen controller.

Component-specific NetworkPolicies default to DNS-only outbound access, except
web-to-control-plane proxy traffic. They do not accidentally select dependency
pods by a shared release label. Configure ingress/monitoring source selectors and
explicit dependency/IdP/upstream/SIEM/OTLP egress peers and ports. Subcharts have
their own network policy settings, which must also allow vMCP clients. Policies
require a supporting CNI. DNS policy assumes kube-system DNS; adapt selectors for
NodeLocal DNS or your cluster architecture. NetworkPolicy filters pods/ports, not
HTTP paths; monitoring peers can reach other API paths on the same port, where
normal application authentication still applies. vMCP's own SSRF egress policy is
a separate required layer for private upstream endpoints.

## Optional bundled dependencies

Overlay `examples/bundled.yaml` to see all four dependency configuration blocks.
It is a template, not runnable HA infrastructure. Set only the dependencies you
actually need. Enabling Kafka/RabbitMQ does not enable an audit exporter, and
selecting an external destination does not require a bundled broker.

Chart versions are pinned in `Chart.yaml`/`Chart.lock`. The pins match PostgreSQL
17, Redis 8, Kafka 4.0 and RabbitMQ 4.1 chart interfaces. Review compatibility and
update deliberately. Bitnami's public image catalog changed in 2025; historical
public version tags can be unavailable or unmaintained. Enabled dependencies
therefore require explicit image digests and stable credential Secret references.
Supply maintained, compatible Bitnami Secure Images or reviewed private mirrors;
do not substitute stock upstream images into Bitnami charts without testing their
entrypoints, paths and environment contracts. Configure all auxiliary images too
if enabling exporters, volume permission jobs or other subchart features.

The bundled example uses placeholder private images and explicitly disables
Bitnami's registry-name verification to demonstrate a reviewed private mirror.
That switch does not establish image compatibility or security. Actual images,
registry access, TLS and dependency installation must be verified in your environment.

Subchart credential Secrets are separate from vMCP connection-URL Secrets. Configure
matching DNS, users, passwords, TLS trust and database name in both. By default a
Service is generally named `<release>-postgresql`, `<release>-redis-master`,
`<release>-kafka`, or `<release>-rabbitmq`; check the rendered output when overriding
names or architecture. The PostgreSQL subchart's single application user is not a
least-privilege role provisioning system: use reviewed initialization SQL or a
separate database provisioning process for production identities.

Use `postgresql.primary.persistence`, `redis.master.persistence`,
`kafka.controller.persistence`, `rabbitmq.persistence`, their storage classes,
resources, placement and TLS/auth options. Single-instance PostgreSQL or Redis is
not HA. Three broker replicas alone do not prove broker durability, quorum queue
setup or failure tolerance. Operate backups/PITR, replication, failover fencing,
PVC lifecycle, topology and recovery drills separately. Avoid automatic stateful
service version upgrades alongside an application upgrade. Removing a dependency
or uninstalling the release does not safely decommission its data: review the
subchart's PVC retention and Secret recovery behavior first.

## Audit destinations and monitoring

Overlay `examples/exporters.yaml` for independent HTTPS, Kafka and RabbitMQ
Deployments. Supply the matching exporter binary image, restricted database LOGIN,
destination configuration and TLS/authentication. Different destinations must use
different stable consumer names; the chart does not enforce uniqueness. Kafka
brokers discovered through metadata must be reachable under both Kubernetes and
application egress policy. Provision Kafka topics and RabbitMQ exchanges/queues
and durable bindings before starting exporters. At-least-once delivery requires
receiver deduplication by event UUID. See `docs/AUDIT_EXPORTERS.md` for all settings.

Overlay `examples/monitoring.yaml` for two ServiceMonitors, the existing alert groups,
and both existing Grafana dashboards. ServiceMonitor/PrometheusRule creation fails
clearly unless their CRDs exist. This chart does not install Prometheus Operator
or Grafana. Configure their label/namespace selectors, scrape access and dashboard
sidecar label. Optional `monitoring.dashboards.namespace` targets an existing
Grafana namespace. Adjust authorization-staleness alerts when changing the runtime
staleness budget. Resources contain the repository's existing monitoring assets;
CI checks that copies do not drift.

The optional tenant router requires its own image, operator-owned directory in
`env.VMCP_TENANT_DIRECTORY`, mounted server certificate/key and corresponding
`VMCP_TENANT_CERT`/`VMCP_TENANT_KEY` paths. It listens with TLS on port 8443 and is
not exposed by the standard console/gateway Ingress. Configure a separate routing
entry point, network permissions and probes. See `docs/TENANT_OPERATIONS.md`.

## Verification and operational limits

```sh
helm dependency build deploy/helm/vmcp
HELM_BIN=helm node --test tests/helm/chart.test.cjs
```

The test suite covers install/upgrade/manual migration rendering, secret stability,
credential isolation, HPA/PDB behavior, networking, CRD checks, monitoring asset
parity, optional dependencies and rejected configurations. CI also builds images
and runs `tests/helm/smoke.py` in a disposable Kind cluster: fresh install, runtime
web configuration, authenticated MCP tool execution, upgrade and rolling restart.
The smoke test explicitly uses local non-production credentials, plaintext test
services and disabled NetworkPolicy; Kind's default CNI does not establish policy
isolation. Optional stateful subcharts are rendered/schema-validated, not certified
for live HA. No chart can establish production RPO/RTO, load capacity or compliance
without evidence from the intended infrastructure.
