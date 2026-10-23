# Isolated MicroK8s load test

Run `python3 benchmark/kubernetes/run.py --smoke`, then
`python3 benchmark/kubernetes/run.py` from the repository root. Requires an existing
single-node MicroK8s, Docker, Go, Kubernetes node-proxy read access and write access
to the local MicroK8s containerd socket for importing test images. No addons,
production namespaces, ingress, registry or existing deployments are modified.
Each invocation creates a fresh `vmcp-load-<timestamp>` namespace, imports locally
built images, and deploys PostgreSQL, Redis, control plane and two data planes.
The namespace and temporary files are removed after evidence collection.

The existing acceptance test's open-loop generator, response/PII/route validation,
nonce duplicate detection, durable audit correlation and cross-node session,
revocation, catalog and snapshot checks are reused. The harness runs the compiled
working-tree data-plane implementation.
The driver and 16 synthetic upstream listeners run on the host; fixtures bind only
to the selected node IP, use synthetic credentials and are closed after the run.
The same no-retry test proxy targets dedicated real NodePort Services, traversing
Kubernetes service/CNI networking. This is **not** an ingress throughput test.

The default REST profile retains 300 upstreams, 300 virtual servers, 60,000 tools,
600 sessions and 64 agent keys. Metadata-only strict audit is enabled; payload
capture and rate limits are not enabled. The generator uses 512 bounded workers
and GOMAXPROCS=8. No race instrumentation is used for capacity measurements.

| Phase | Target calls/s | Seconds |
|---|---:|---:|
| Warmup | 50 | 30 |
| Compose-rate baseline | 200 | 90 |
| Steady | 600 | 300 |
| Spike | 2000 | 60 |
| Recovery | 600 | 45 |
| Config propagation | 600 | 30 |
| One pod withdrawn | 600 | 90 |
| Pod rejoins | 600 | 60 |
| Soak | 400 | 1800 |

This is approximately 42 minutes of traffic, plus setup and verification. The
pod fault scales only the owned data-plane Deployment to zero and restores it;
it is not a physical cluster-node failure and does not guarantee abrupt SIGKILL.
The unchanged acceptance thresholds are declared by the driver before traffic.
A failed performance gate still saves evidence and returns nonzero.

Per data-plane pod: GOMAXPROCS=2, GOMEMLIMIT=768MiB, pool=64, requests=2CPU/2GiB,
limits=4CPU/3GiB. PostgreSQL requests=2CPU/2GiB, with no CPU or memory limits; Redis
limits=1CPU/512MiB. The control plane is a separate pod (1CPU/1GiB limit).
Kubelet RSS and recent CPU are sampled about every five seconds, together with
application counters and PostgreSQL locks/connections/deadlocks/size. Metrics
sampling failures are visible, and missing per-node RSS fails the run. Evidence
includes source-tree hash, binary hash, image IDs, resource manifests, pod/events,
logs and report JSON/Markdown/SVG. No database credentials/master key are exported
in the runtime manifest or report (the seeded runtime Secret is not collected).

Long-run overrides are `VMCP_ACCEPTANCE_RPS`, `VMCP_ACCEPTANCE_BURST_RPS`,
`VMCP_ACCEPTANCE_STEADY_SECONDS`, `VMCP_ACCEPTANCE_SPIKE_SECONDS`,
`VMCP_ACCEPTANCE_SOAK_RPS`, `VMCP_ACCEPTANCE_SOAK_SECONDS`,
`VMCP_ACCEPTANCE_WORKERS`, and the inventory options from `../acceptance/README.md`.
Raw evidence defaults to the ignored `artifacts/benchmark-runs/microk8s-<timestamp>/`.
At completion (including failed runs), a verified compressed archive is saved under
`artifacts/benchmark-archives/` and small reviewable summaries are exported to
`benchmark/results/microk8s-<timestamp>/`. Use `--output <new-directory>` to choose
a different raw evidence directory; the curated export still uses the run timestamp.
Do not commit the raw directory. See the [results policy](../results/README.md). Interrupting the
Python runner performs namespace cleanup; killing it with SIGKILL cannot do so.
Historical Compose and Kubernetes measurements differ in platform/resource limits;
use the 200RPS baseline and the saved environment when interpreting comparisons.

The 2026-10-23 measured run and its Compose comparison are in
[`../results/microk8s-20261023-193023/comparison.md`](../results/microk8s-20261023-193023/comparison.md).
It did **not** meet the high-load acceptance thresholds. The successful smoke run
is `../results/microk8s-20261023-192851/report.md`.

## Focused PostgreSQL diagnosis

`python3 benchmark/kubernetes/run.py --database-focus` keeps the same inventory,
two data-plane pods, and strict durable audit. After 15 seconds at 50 TPS it offers
1,000 TPS for 300 seconds. No fault or configuration mutation occurs during load;
post-load audit, session termination and key revocation checks still run. Before
traffic it inserts 100,000 expired operations and streams into the isolated test
database, so retention cleanup is exercised without waiting 15 minutes. These
fixtures have no audit admissions and are excluded from client success counts.

PostgreSQL has **no CPU/memory limits**. This change only affects newly created
benchmark namespaces, not an existing deployment or production Helm settings.
Resource requests remain 2 CPU/2GiB. `pg_stat_statements.track=all`, I/O and WAL
I/O timing are enabled. An observer samples activity, blocking PIDs, tables,
indexes, WAL and expired-operation backlog roughly every five seconds; statement
snapshots and cgroup counters roughly every 15 seconds. SQL timing includes waits
and nested statements, so totals are not additive CPU time. Diagnostic overhead
is present and must be considered when comparing older non-instrumented runs.

Each run also executes the new migration upgrade/rollback/cascade test and the
existing durable delivery tests against the disposable Kubernetes database.

Use `--data-plane-cpus 8` for a controlled application-CPU comparison (default
2; supported 2/4/8). This changes GOMAXPROCS and CPU requests/limits for the two
owned data-plane pods. PostgreSQL and the inventory/profile remain unchanged.
Run comparisons **sequentially**, so they do not compete on the same host.
The accepted traffic rate still depends on the measured successful responses,
not the offered rate. PostgreSQL NOTIFY waits can occur during COMMIT outside
what the `pg_stat_statements` entry for `pg_notify` itself appears to cost; use
activity/lock samples together with SQL timings.

The two sequential 2026-10-23 five-minute diagnostic runs and proposed fixes are
in [the PostgreSQL diagnosis](../results/microk8s-postgres-1000-20261023-analysis.md).
Both missed the 1,000 TPS capacity/latency gate. The report separates successful
throughput, dropped demand, PostgreSQL notification-lock waits, cleanup plans,
and the application CPU comparison; migration/delivery integration tests passed.

## Polling and PostgreSQL profile comparison

Delivery now wakes in-process readers locally and uses polling between replicas.
A focused run also probes remote delivery during the first load window: the host
writes 24 small durable events and a real pod serves them through GET SSE. It
records commit/write-start-to-client-read p50/p95/max and fails its separate gate
if any sampled event exceeds 500ms. This is a limited sample, not a latency SLA.
It uses a dedicated GET stream within an existing synthetic session, so it does
not change tool success or duplicate/audit counts.

For a controlled comparison run sequentially:

```sh
python3 benchmark/kubernetes/run.py --database-focus --data-plane-cpus 8
python3 benchmark/kubernetes/run.py --database-focus --data-plane-cpus 8 --postgres-tuned
```

`--postgres-tuned` applies the settings in `postgres_profile.py`: bounded 1GiB
shared buffers, existing 4GiB planner cache estimate, 32MiB WAL buffers, 1/4GiB
min/max WAL, 15-minute checkpoints and more frequent vacuum/analyze. Work memory
stays 4MiB per operation; the connection budget stays 256. It mounts 512MiB tmpfs
at `/dev/shm`. `fsync`, synchronous commit and full-page writes stay enabled;
WAL level, storage class, CPU/memory limits and application pool size do not change.
The effective pg_settings values are collected, rather than assuming requested
configuration took effect. These are local benchmark choices, not universally
optimized production values. A 5-minute run does not exercise repeated 15-minute
checkpoints or a complete fresh delivery retention cycle. Shared-host sizing and
production persistent storage require their own longer validation.

Optional `--postgres-explain` samples 1% of statements with execution time ≥10ms
using auto_explain (ANALYZE/BUFFERS on, per-node TIMING off). Plans are in
`postgres.log` in the local raw evidence/archive. This diagnostic adds overhead and should be recorded when
comparing runs. It is only configured on the disposable benchmark PostgreSQL.

The polling implementation and repeated five-minute 1,000 TPS runs are documented
in [the polling/PostgreSQL report](../results/microk8s-polling-1000-20261023-analysis.md).
The optimized runs pass the steady capacity/latency gate; the final run also
passes the sampled 500ms remote-polling gate. Overall acceptance remains nonzero
because the existing large-catalog key-revocation propagation check fails. The
report preserves the failed baseline and first probe setup failure as evidence.
