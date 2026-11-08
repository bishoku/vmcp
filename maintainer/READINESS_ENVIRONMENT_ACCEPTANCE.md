# Environment acceptance — issue #55

Status: partial acceptance; reference capacity failed; recovery checks passed; environment-dependent items remain open.

Source: PR #56, `a69649ff72908f05f450b4a9b7d57ae4c03edf0f`.
No merge, release or production deployment is authorized. SAML/SCIM are excluded.

## Scope and method

Use `benchmark/kubernetes/run.py` with a fresh `vmcp-load-*` namespace for each
run. PostgreSQL, Redis and application replicas run inside that namespace.
Docker is used only to build/save images for import; no Compose fixtures or
per-run Docker bridge networks are created. Existing deployments are read-only.
Raw evidence is retained outside Git under `artifacts/benchmark-runs/`.

Each result must include source/binary/image identities, resource topology,
reproduction command, actual duration, offered/completed traffic, errors, dropped
demand, latency and integrity evidence. Functional harness gates and diagnostic
load measurements are distinct from user-approved capacity acceptance.

The user selected the latest load-test profile as the capacity reference: 1,000
offered calls/s for 600 seconds, at least 99% completion of target, 99.9% success,
p95 <=250 ms and p99 <=1,000 ms. RPO/RTO targets remain unspecified.
Historical measurements are not current acceptance results. Single-node pod
withdrawal is not physical-node HA; single PostgreSQL restart is not promotion;
logical backup restore is not PITR; a fixture sink is not independent WORM.

## Reference profile

The user selected the most recent load-test profile as the configuration reference.
`artifacts/benchmark-runs/issue31-1000tps-bulk/{profile,manifest,postgres-profile}.json`
records: one DP, 8 CPU/GOMAXPROCS, 4GiB Go memory target, 6Gi pod limit, pool 64;
300 virtual servers × 200 tools, 600 sessions, 1,024 workers, 1,000 offered calls/s
for 600 seconds, strict metadata audit and ownership. PostgreSQL uses the existing
`--postgres-tuned` profile with fsync/synchronous_commit/full_page_writes enabled.
This reference supplies settings and harness gates, not current measured capacity.
HA must be evaluated separately with at least two replicas at the same per-replica
resource budget. Physical node HA remains outside the single-host topology.

## Reference capacity result

**Failed.** Run `artifacts/benchmark-runs/readiness-55-reference-4g6g/`,
2026-11-08 18:28:07 UTC start; complete Go test 711.93 seconds; steady phase
600.078 seconds. Source/application SHA is the baseline above; harness-only
changes were uncommitted and captured in `environment.json` workingTree.
Source-tree hash: `7d098b6f2b42857b6e27ad218a5185b273f610c7c60ff239aa0b7b269b9d93ae`.
DP image index digest: `sha256:68a0298b15deef684253cd9fed42ed4629084412a5aaf55a020279f6918e9639`.
DP binary SHA256: `e3095392a1938688a3fd6ef5cd425b176f1c8e05b1a039a8809f8ced402ebd6c`.

| Measurement | Target | Observed | Result |
| --- | --- | --- | --- |
| Successful completion | >=990/s at 1,000 offered/s | 982.891/s; 589,811 successful; 599,990 offered of 600,000 scheduled | Failed |
| Success | >=99.9% | 98.303%; 10,179 errors; 10 dropped demand | Failed |
| Latency | p95 <=250ms, p99 <=1,000ms | 181.406ms / 494.225ms | Passed for this phase |
| RSS/container usage | <=5 GiB / <=5.4 GiB | 2,269,331,456 / 2,286,415,872 bytes | Passed |
| RSS growth | <=20% | 15.831% | Passed |
| Loaded revocation | <=5s, readiness healthy, rejection stable | 3.819s; healthy/stable | Passed |
| Admission/outcome integrity | No duplicates or unexplained missing outcomes | 590,561 successful calls including warmup; exactly matching admissions and ok outcomes; zero duplicates/incomplete | Passed |

2 HTTP 502 and 10,008 HTTP 503 responses follow two NodePort connection refusals
and the test proxy's passive 10s cooldown. Another 169 HTTP-200 MCP errors remain
unattributed. All errors remain in the failed result. See
[database cost review](READINESS_DATABASE_COST_REVIEW.md) for WAL waits, SQL cost,
application limits and proposed work; no production performance change was made.

Additional observed resource signals: PostgreSQL max sampled CPU 502.976%
(about 5.03 cores), RSS 1,602,764,800 bytes. The last retained admission-lock metric
sample has 587,237 calls, zero errors and 52.704 cumulative seconds (about 0.090 ms
per call, including the query exchange). Maximum sampled oldest incomplete audit
age was 0.564s; sampled audit queue pending was zero. This is local audit evidence,
not external exporter lag. GC traces are retained; no GC latency SLO was supplied.
Short calls do not establish long-operation lease renewal behavior or breaker
probe latency under sustained failure. Database disk latency is inferred from WAL
wait/sync evidence; no independent block-device latency experiment was run.

Evidence: report.json, run.log, metrics-*.prom, postgres-samples.jsonl,
postgres-statements-*.json, postgres-effective-settings.json, cgroup-database-*.prom,
pod/events JSON and application GC logs in the run directory. Curated summary:
`artifacts/benchmark-summaries/microk8s-20261108-212735/`.
Archive `readiness-55-reference-4g6g-73ee557bc794.tar.gz`, SHA256
`1d459845040cd8bcca2bb7e1fee17b9104d784df0593bf7d9bc8393ba1d21361`.
Archive selection is documented by the publisher; the original run directory
retains all metric samples. Recovery keys are not part of the evidence archive.

Reproduce:

```sh
VMCP_ACCEPTANCE_SERVERS=300 VMCP_ACCEPTANCE_TOOLS=200 \
VMCP_ACCEPTANCE_NODES=1 VMCP_ACCEPTANCE_ROUTING=nodeport \
VMCP_ACCEPTANCE_RPS=1000 VMCP_ACCEPTANCE_STEADY_SECONDS=600 \
VMCP_ACCEPTANCE_WORKERS=1024 VMCP_ACCEPTANCE_OWNERSHIP=1 \
VMCP_ACCEPTANCE_DIAGNOSTICS=1 VMCP_ACCEPTANCE_GOMEMLIMIT=4GiB \
VMCP_ACCEPTANCE_MEMORY_PROFILE=4g6g \
VMCP_RECOVERY_PG_BIN=/usr/lib/postgresql/17/bin \
python3 benchmark/kubernetes/run.py --database-focus --data-plane-cpus 8 \
  --data-plane-memory-limit 6Gi --postgres-tuned --recovery-checks \
  --output <new-directory>
```

Remaining acceptance work references #55 rather than closing it.

## Environment and coverage boundaries

Read-only inventory is retained in
`artifacts/benchmark-runs/readiness-environment-20261108/inventory.json`.
Existing demo deployments have different pinned image digests; their source SHA
was not established and they were not used as the acceptance target. Their CP/DP
limits are 1 CPU/512Mi and DP replica count 2, unlike the chosen benchmark profile.
Secret names/references and environment variable names were inventoried without
reading key material. Existing backup recovery chains and external TLS/WORM
configuration were not certified by that inventory.

Fresh fixtures use one MicroK8s v1.35.6 node, 64 logical CPUs and approximately
157 GiB RAM, containerd 2.1.6. All replicas and the native load driver share this
host with other workloads. Each run owns its `vmcp-load-*` namespace and synthetic
databases. PostgreSQL 17 is a single primary with ephemeral emptyDir; Redis 7 is
single instance, 1 CPU/512Mi limit. PostgreSQL has no CPU/memory limit and uses the
reference tuned settings. No standby, archive recovery chain or independent
immutable audit sink is provisioned in these fixtures. NodePort HTTP excludes
production ingress/TLS overhead. Strict metadata audit, local snapshot
configuration and generated per-run encryption keys are used. No existing
installation is upgraded; rollback is deletion of the newly created namespace.

## Logical backup and key recovery

**Passed within the fixture scope.** The reference command's `--recovery-checks`
runs `TestLogicalRecoveryAcceptance` after load against two distinct disposable
databases in the same isolated PostgreSQL fixture. PostgreSQL 17 pg_dump creates a
real custom-format backup; pg_restore restores it into the other database. Two
configuration rows encrypted with old/new keys both decrypt with the retained
keyring. Wrong-row AAD and cross-tenant decryption fail as required.

Test duration 3.11s; backup 0.237s; restore and decrypt 1.129s; dump 249,543 bytes.
Backup SHA256 `e49fbc9896f576b6d5b7c86e5cd42bab2dd1f5d08a16cb885c3e3815049f9091`.
Raw evidence: reference run `recovery-tests.jsonl`, `recovery/recovery.json` and
`recovery/fixture.dump`. Keys retained separately under
`local/readiness-recovery-keys-20261108-212735/`, excluded from Git. These times are
not application RTO, production RPO, cross-host recovery or physical PITR.
Application HTTP behavior after restore and legacy-backup migration were not
exercised by this restore test.

## Correctness and exporter component checks

**Passed within component/integration scope.** `correctness-tests.jsonl` records
13 passing race-enabled tests: bound/tenant envelope rejection; reader-first key
rotation/recovery; explicit legacy opt-in and disabling; restartable rotation;
durable routing/fencing/replay/isolation; expired worker rejection; scoped resource
hints/revocation; two warm-node grant/credential revocation; exporter lost
acknowledgement with independent consumer ledgers; dead-letter/expired lease
recovery; stable payload retry after expiry. The runner records the exact command.

The exporter test delivers the same event ID twice after lost acknowledgement to
one fixture publisher and once to the other; retention remains until both have
receipts. This proves ledger/retry behavior, not independent sink deduplication,
production backlog-drain capacity or WORM retention. Full-deployment exporter
outage/recovery and independent WORM permission/legal-hold acceptance remain
**not run**. No retention lock was created or weakened.

## Remaining acceptance items

| Item | Status | Limitation / next required evidence |
| --- | --- | --- |
| 600s distributed reference capacity | Failed | Diagnose routing and MCP errors; compare DB storage using cost review before performance changes |
| Hot virtual server sustained capacity | Not run | New load runs deferred in favor of the requested low-load SQL investigation |
| Full MCP/REST × short/long SSE × PII/payload matrix | Not run | Short REST-upstream MCP calls and separate correctness checks do not establish this matrix |
| Multi-replica pod withdrawal | Passed within harness scope | Two replicas, same host, short calls; 8 fault-phase errors retained; no physical-node HA/RTO claim |
| Rolling restart with long in-flight operations | Not run | Requires dedicated lifecycle/outcome accounting; scale withdrawal alone is insufficient |
| Physical node failure | Not run | Single physical node; unrelated workloads must not be disrupted |
| PostgreSQL primary promotion/fencing | Not run | Single-primary fixture has no standby; restart would not prove failover |
| PostgreSQL PITR | Not run | No physical base backup + WAL archive chain in fixture |
| Logical encrypted fixture restore | Passed | Does not include restored application HTTP exercise |
| Legacy opt-in → rotate → restore → disable | Not run end-to-end | Individual rotation/legacy tests passed; full restored chain still needed |
| Independent WORM retention/legal hold/role separation | Not run | No verified independent sink; no irreversible lock authorized |
| Production RPO/RTO | Not run | Targets and representative recovery topology still required |

Release readiness remains unproven. Priorities are P0 failure attribution and
unmeasured recovery guarantees; P1 production-like DB comparison, lifecycle HA
and exporter/WORM acceptance; P2 optional Redis hints and query-exchange reductions
only after correctness-preserving measurements. Refs #55; do not close the issue.

## Two-replica lifecycle and overload diagnostic

**Passed against existing harness phase gates**, not a 600-second capacity or
physical-node HA certification. Run `artifacts/benchmark-runs/readiness-55-ha-4g6g/`,
namespace `vmcp-load-20261108-214313`; 615.52s Go test. Same application SHA and DP
binary as reference; source-tree hash
`1bd558d380e64f5be557dc7ec2ea3f96bd0047b091e6501ff06b84628a8a86a9`.
DP image index `sha256:04df97b7d4b79d5b97892dc69f51b24af3108846dc34a0e755925f474b74388c`;
running pod imageID `sha256:6d9e19de6a1bf22fdf5526d95ff1613dd5ed3bc274fee4cd7d723e1231e3666f`
in `pods-before-load.json`. Two replicas each have 8 CPU, 4GiB Go target, 6Gi limit,
64 pool. PostgreSQL/Redis/host topology is unchanged from the reference fixture.

```sh
VMCP_ACCEPTANCE_SERVERS=300 VMCP_ACCEPTANCE_TOOLS=200 \
VMCP_ACCEPTANCE_NODES=2 VMCP_ACCEPTANCE_ROUTING=nodeport \
VMCP_ACCEPTANCE_RPS=1000 VMCP_ACCEPTANCE_BURST_RPS=2000 \
VMCP_ACCEPTANCE_WARMUP_SECONDS=15 VMCP_ACCEPTANCE_STEADY_SECONDS=60 \
VMCP_ACCEPTANCE_SPIKE_SECONDS=20 VMCP_ACCEPTANCE_SOAK_SECONDS=60 \
VMCP_ACCEPTANCE_WORKERS=1024 VMCP_ACCEPTANCE_OWNERSHIP=1 \
VMCP_ACCEPTANCE_DIAGNOSTICS=1 VMCP_ACCEPTANCE_GOMEMLIMIT=4GiB \
VMCP_ACCEPTANCE_MEMORY_PROFILE=4g6g-ha \
python3 benchmark/kubernetes/run.py --data-plane-cpus 8 \
  --data-plane-memory-limit 6Gi --postgres-tuned --output <new-directory>
```

| Phase | Actual seconds | Successful calls/s | Success % | p95 / p99 ms | Errors / dropped |
| --- | ---: | ---: | ---: | ---: | ---: |
| warmup | 14.997 | 50.010 | 100.0000 | 23.027 / 32.234 | 0 / 0 |
| baseline | 90.028 | 199.937 | 100.0000 | 38.751 / 51.678 | 0 / 0 |
| steady | 60.041 | 999.241 | 100.0000 | 81.201 / 105.092 | 0 / 5 |
| spike | 20.354 | 1848.179 | 94.1957 | 589.175 / 617.927 | 2318 / 64 |
| recovery | 45.052 | 998.853 | 100.0000 | 79.360 / 106.460 | 0 / 0 |
| config-refresh | 30.039 | 997.911 | 99.9200 | 81.687 / 106.981 | 24 / 0 |
| node-loss | 90.081 | 999.009 | 99.9911 | 184.993 / 304.282 | 8 / 0 |
| node-rejoin | 60.054 | 999.097 | 100.0000 | 66.175 / 95.828 | 0 / 0 |
| soak | 60.038 | 399.749 | 100.0000 | 56.358 / 75.350 | 0 / 0 |

The steady gate is >=99% target completion, >=99.9% success, p95<=250ms,
p99<=1,000ms. Existing overload gates allow >=80% completion/success and
p99<=10s. Existing fault gate allows >=99% success with a 10s transition grace;
that grace is a harness policy, not an agreed production RTO. Report.json retains
all phase errors, seconds and thresholds. No RPO measurement is made.

All 365,331 successful calls matched admissions and outcomes; zero duplicate
executions or incomplete operations. Max RSS: 2,065,887,232 and 2,026,438,656 bytes;
max container usage: 2,082,447,360 and 2,043,342,848 bytes. Pool maximum acquired
36/64 and 64/64, no cancelled acquires. Config refresh and post-load revocation/
session termination checks passed on both replicas. Long-running ownership loss
is only covered by the separate correctness tests, not this short-call fault run.
The fault scales one test Deployment to zero and restores it; it does not perform
a full rolling restart or destroy a physical node.

Raw commands, durations, metrics/GC, pods, events and report are retained in the
run directory; curated output `artifacts/benchmark-summaries/microk8s-20261108-214313/`.

HA raw archive: `readiness-55-ha-4g6g-77c911055944.tar.gz`, SHA256
`92a47e4fe9707b61a88d3f6048547ca44d6a6767b5babb58c81de04a35972327`.

## Low-load SQL-cost acceptance

**Passed as an accounting/correctness fixture, not a capacity gate.**
`artifacts/benchmark-runs/readiness-55-sql-cost/`, namespace
`vmcp-load-20261108-215423`; TestSQLCostAcceptance 9.21s. Application source SHA is
a69649ff72908f05f450b4a9b7d57ae4c03edf0f; source-tree hash
`3cf23e16dfe886f0f3f93164bc175b50b32bfd936984db272932fd925bc0d443`;
DP image index `sha256:acd4657f0c6db75cde67029d59a9c7751bf112d579e0082a66db0a5e52e02051`.
The same application implementation runs with new diagnostic test sources.
One active DP, 8 CPU, 4GiB Go/6Gi container, PostgreSQL tuned, single Redis,
one initialized session, one virtual server, two catalog tools. Request traffic
is one warmup + one measured + twenty serial calls to a REST upstream with PII
and strict metadata audit. All 22 are successful with exact audit matching.

```sh
VMCP_ACCEPTANCE_GOMEMLIMIT=4GiB VMCP_ACCEPTANCE_OWNERSHIP=1 \
python3 benchmark/kubernetes/run.py --sql-cost --postgres-tuned \
  --data-plane-cpus 8 --data-plane-memory-limit 6Gi --output <new-directory>
python3 benchmark/kubernetes/sql_cost.py <new-directory>
```

Observed request path: 29 top-level commands/call (including eight transaction
boundaries), plus 18 nested trigger/FK statements. Background SQL is separated
by query identity, and no shared counters are reset. See the database cost review
for the complete accounting, idle cost and proposed changes. These measurements
have no latency/throughput acceptance target.

Raw evidence: `sql-{idle,single,serial}-{before,after}.json`, `sql-cost.json`,
`run.log`; derived `sql-cost-deltas.json` is reproducible offline. Curated summary
`artifacts/benchmark-summaries/microk8s-20261108-215423/`; raw archive
`readiness-55-sql-cost-ee1e2f9e1f56.tar.gz`, SHA256
`f078cab2bdfbbf4a89cfcba313a9b899a4d5b67c52511aec9241e1e7a5776a37`.

## Cleanup and handoff

All three campaign namespaces were deleted through normal cleanup. Final
read-only check found the node Ready, all listed existing deployments at desired
ready replica counts, and no `vmcp-load-*` namespace remaining. Only newly created
fixture databases/namespaces and temporary build directories were removed;
backups, recovery keyrings, raw evidence and archives remain local. No merge,
release, production deployment, paid resource or immutable retention change was
performed. The separate research report proposes changes; it does not implement
Redis offload or modify production SQL/lease/audit semantics.
