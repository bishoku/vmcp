# Dedicated tenants and isolated stdio workers

## Tenant isolation model

vMCP hosts multiple tenants using **dedicated deployments and databases**, routed
by exact public hostnames. It does not offer shared-schema tenant switching.
Each tenant has its own control/data plane processes, PostgreSQL database and
LOGIN identities, encryption/cookie keys, OIDC client/audience/bindings, worker
host, egress grants and audit exporter identity. This deliberately makes missing
row predicates incapable of selecting another tenant's database. Shared-schema
RLS is unnecessary for this model and is not claimed as implemented.

Set `VMCP_TENANT_ID` consistently on every plane and operational command. Run
`cmd/migrate` with that ID before launching a new dedicated tenant. A database
identity is bound only while empty; it cannot be reassigned by changing the
runtime environment. Startup rejects a different ID or multiple tenant rows.
Existing deployments retain ID `default` and their internal row relationships.
The internal configuration tenant row remains `default`; the authoritative hosted
tenant boundary is its immutable database binding, not a request-selected SQL
predicate. API `/info`, Settings and exported audit rows identify the hosted tenant.

`cmd/tenant-router` consumes an operator-owned `VMCP_TENANT_DIRECTORY` JSON array
of `{id,host,control,data,ui}`. All target origins require HTTPS; tenant plane
origins cannot be shared. Its HTTPS listener uses `VMCP_TENANT_CERT/KEY`.
Unknown hosts fail, forwarded tenant headers are replaced, and each tenant plane
still authenticates credentials independently. DNS/TLS certificates and ingress
must cover exactly the approved public hosts. Tenant onboarding is an operator
workflow; there is no API to register arbitrary routing destinations. Egress
controls apply to the router. Prefer an independently operated ingress with the
same exact-host routing if it already provides this functionality.

Deploy one Helm release/namespace per tenant and apply separate network policies.
Revoke PUBLIC database CONNECT privileges and grant each tenant LOGIN access only
to its own database; never reuse migration-owner credentials as runtime identities.
Use distinct master/recovery keys even though new ciphertext also authenticates
the configured tenant context. Tenant IDs cannot be renamed after storing data
without an explicitly designed re-encryption migration. Legacy unversioned
ciphertext lacks tenant context and must be rotated before sensitive workloads.
Cookie signing and Redis bucket prefixes include tenant identity. Per-process
caches, session stores, snapshots and egress grants never serve another tenant.
Redis DBs/ACLs and exporter destinations must also be isolated where operators
or tenants can directly access those services. Limit resource quotas per tenant;
the router does not allocate shared process/database capacity between tenants.

No deployment can protect against an operator deliberately giving tenants the
same keys, database privileges or identity-provider audiences. Infrastructure
ACLs, independent tenant identities and backup/export access reviews are required
release gates. Restore into the original tenant identity, retain its keys and
verify the database binding before exposing the service. Direct plane endpoints
must remain private to the approved ingress/router.

## Stdio worker boundary

The `stdio_worker` upstream transport connects over HTTPS/mTLS to
`https://worker-owner/mcp/PROFILE`. It never starts a process in an API plane.
Configure client certificate/private key and CA on the upstream. Fallback URLs
are disallowed because physical sessions belong to a single worker owner.
Each trusted gateway client certificate can access every profile on that worker;
use a dedicated worker/CA per tenant and do not share it between trust domains.

Run `cmd/stdio-worker` on a separate host with its own rootless Docker daemon.
Never mount that daemon into control/data-plane pods. Configure
`VMCP_STDIO_PROFILES` as a JSON object mapping profile names to `{image}`;
images must be repository digests or immutable local `sha256:` image IDs.
Pre-pull reviewed images: workers use `--pull=never`. Image entrypoints are the
operator-controlled commands; no user-supplied executable, shell, argument,
host volume, inherited child environment or network namespace is accepted.
Optional operator-owned `secretFile` profile names resolve under
`VMCP_STDIO_SECRET_ROOT` (default `/run/vmcp-secrets`). Files must be private regular
files with no group/world permissions; their Docker env-file contents are injected
only into that profile container. Never expose the secret directory to tenants.
Rotation takes effect on new sessions; terminate old sessions when required.
`VMCP_STDIO_CLIENT_CA`, `VMCP_STDIO_CERT` and `VMCP_STDIO_KEY` are required.
The server requires verified client certificates and TLS 1.3.

Containers run as UID/GID 65532 with no capabilities, no privilege escalation,
no network, a read-only root filesystem and a 16 MiB noexec/nosuid temporary
filesystem. Limits are 128 MiB memory (no additional swap), 0.5 CPU and 64 PIDs.
The worker caps sessions at 32, HTTP handlers at 64, request bodies at 1 MiB, event queues at one frame,
stdin writes at 5 seconds, initialization at 10 seconds and requests at 65
seconds. Stdout frames are bounded at 2 MiB; the worker discards Docker CLI stderr
rather than mixing it into MCP frames. Docker daemon logging can independently
retain container stdout/stderr; disabling payload capture is still a production
roadmap item. Idle sessions expire at 15 minutes. Container
termination explicitly removes the named container, and worker shutdown closes
all owned sessions. There is no automatic unsafe tool replay or process restart.

Placement uses a stable **owner endpoint**, not an unfenced load-balanced worker
pool. Different worker hosts have different URLs. A worker crash loses its
in-memory session registry and returns unknown sessions after restart. Daemon-owned
containers are not guaranteed to stop on abrupt process death; startup orphan
reconciliation remains a production roadmap item. Clients create
fresh sessions, and ambiguous unsafe operations require operator/client review.
Gateway replay remains durable, but stdio process state itself is not migrated.
Images needing network or host files are intentionally unsupported by this
first isolated profile. Approved network access requires a separate operator
policy and escape-test milestone.

Regression tests cover profile/session isolation, immutable allowlists, lifecycle,
real container UID, denied root writes, denied network and Docker resource limits.
The fixture is `cmd/fixture-stdio` with `deploy/Dockerfile.stdio-fixture`; set
`VMCP_TEST_STDIO_IMAGE` to its immutable image ID for the real Docker check.
These tests do not prove resistance to a container-runtime/kernel escape; keep
the worker host patched, use rootless isolation and assess stronger sandboxes
before running hostile third-party binaries.

Reserve at least 1 GiB for the worker service and 4 GiB for its maximum 32
containers, then measure actual workloads. Output queue backpressure is bounded
at five seconds; a stuck consumer terminates the physical session. Worker frame
and request limits are intentionally stricter than general HTTP upstream limits.

The standard distroless API image intentionally has no Docker CLI/socket. Run the
released `stdio-worker` binary on its dedicated worker host with the separately
provisioned Docker CLI/daemon; do not build it with the standard API Dockerfile
and expect process launch to work. Release binaries include the worker and router.
