# PostgreSQL cost and optional Redis offload — Refs #55

## Decision

Do not optimize application semantics to compensate for this host's storage.
The current evidence points to the PostgreSQL WAL flush path as a substantial
latency contributor, rather than Go CPU/memory exhaustion or expensive delivery
query execution. It does **not** establish a hard PostgreSQL throughput ceiling.
The failed 1,000 TPS acceptance also contains a separate test-proxy availability
failure. Neither finding proves the other caused it.

The initial review below describes the unmodified baseline. The follow-up section
records implemented SQL and idle-poll reductions. Durability settings, authorization
freshness, heartbeat intervals and lease guarantees remain unchanged. Redis options
remain proposals, not implemented settings.

## Evidence and limits

Source application: `a69649ff72908f05f450b4a9b7d57ae4c03edf0f`.
Raw run: `artifacts/benchmark-runs/readiness-55-reference-4g6g/`.
Use `report.json`, `postgres-samples.jsonl`, `postgres-statements-latest.json`,
`postgres-effective-settings.json`, `run.log` and the metrics/GC files together.
The load is short MCP tools/call traffic to synthetic REST upstreams, distributed
across 300 virtual servers, with strict metadata audit and durable MCP delivery.
It is not a measurement of every supported MCP workload.

* 600-second phase: 982.891 successful calls/s; 98.303% success; p95 181.406 ms,
  p99 494.225 ms; 10,179 errors and 10 dropped scheduled calls. Acceptance failed.
* Two NodePort connection refusals triggered the harness's passive 10-second
  cooldown: 2 HTTP 502 and 10,008 HTTP 503 responses. A further 169 errors arrived
  with HTTP 200 (MCP/application errors). Their timing coincides with the dedicated
  revocation exercise; retained error examples do not identify their exact cause.
  Do not silently exclude these errors or call the remaining interval a pass.
* Go maximum CPU was 217.775% (about 2.18 cores, with an 8-core limit), maximum RSS
  2,269,331,456 bytes, maximum container usage 2,286,415,872 bytes, no restart/OOM.
  Pool maximum acquired 58/64 and cancelled acquisitions 0. These are samples,
  not proof that every short-lived queue was absent.
* In 107 PostgreSQL samples labelled `steady`, 83.17% of **active-backend sample
  observations** were `WALWrite` or `WalSync`. This is not 83.17% of request time.
  One sample had 63 active backends waiting on `WALWrite`. No blocking-session
  records were captured; row/advisory lock contention is not the dominant
  observed distributed-load wait.
* Between the first/last steady samples (599.324 seconds), WAL grew by
  4,029,043,982 bytes; 234,686 sync operations accumulated 571,252.321 ms,
  averaging 2.434 ms/sync. This interval includes background work and is slightly
  shorter than the load phase. WAL flush bandwidth and flush latency are distinct:
  roughly 6.7 MB/s of WAL does not make the sync path inexpensive.
* Core delivery statement mean execution times were 0.005 ms for advisory lock,
  0.167 ms for active-operation counting, 0.193 ms for fenced event-bound checks,
  0.147 ms for stream insert and 0.158 ms for event insert. These means are not
  end-to-end transaction latencies or p99. Audit bulk inserts have larger means
  but process many events in each statement; compare per event, not per statement.

This PostgreSQL is a single pod, ephemeral emptyDir on the shared host filesystem,
with no standby or dedicated storage acceptance. It already uses the historical
OLTP tuning profile, including 1 GiB shared_buffers, 4 GiB effective_cache_size,
32 MiB wal_buffers, 1/4 GiB min/max WAL and 256 max_connections. fsync,
synchronous_commit and full_page_writes remain on. A production-like DB/storage
comparison should precede application changes motivated only by this latency.

## What one short tool call asks PostgreSQL to do

This is a code-path accounting, corroborated by aggregate counts, **not an isolated
single-request trace**. It assumes an initialized session, one terminal result,
no progress/server requests/replay, and no lease tick during the operation.

| Stage | Application SQL/protocol commands | Purpose |
| --- | ---: | --- |
| Session resolve | 1 SELECT | Authoritative termination/ownership check, even with warm local state |
| Delivery Begin | 5 SQL + BEGIN/COMMIT = 7 | Per-server advisory lock; expire same request slot; check active budgets; insert operation and stream |
| Stream Claim | 1 UPDATE | Fence the response-stream owner |
| Terminal Append | 3 SQL + BEGIN/COMMIT = 5 | Lock/check operation fence and event bounds; insert encrypted event; mark operation done atomically |
| Read Events | Typically 2 SELECTs in this run | Initial empty read plus notified terminal read; count varies with timing |
| Stream Release | 1 UPDATE | Release the response-stream owner |
| Subtotal | Approximately 17 client commands, including 4 transaction boundary commands | Excludes audit, background work and optional paths |
| Strict audit | 2 durable logical events | Admission must persist before upstream execution; outcome persists separately; grouped with other calls |

The run records about 1.00 Begin/Append/Claim/Release per successful call,
2.04 event reads and 1.34 session reads. Failed attempts, setup, verification,
longer-lived request checks and background work are included in these ratios.
They must not be advertised as exact single-call counts.

Audit already groups up to 64 events: event insert, access-context insert and
access-domain insert share a transaction; dependent inserts use pgx batching.
Payload capture adds a payload insert. Under low concurrency the single-event
path applies, so a low-load trace can show **more audit commands per event** than
the loaded grouped path. There is no fixed total SQL count valid for both cases.

`pg_stat_statements.track=all` also counts trigger/function SQL and FK checks.
For example the approximately two tenant identity/access-context checks per call
are database-side checks, not two additional application network round trips.
Do not sum nested and parent execution times, or label all recorded statements
as application calls to PostgreSQL.

Long operations add an operation renewal each second, stream owner/session checks
approximately each second, and event polling every 250 ms when not locally woken.
Progress adds audited, encrypted, fenced event persistence. Client requests add
request rows and a 50 ms reply polling loop. These costs require their own profile.

Code references: `core/internal/delivery/store.go` (`Begin`, `Append`, `Events`,
`Renew`, `WaitReply`), `core/internal/virtual/streams.go` (`streamCall`,
`serveStream`), `core/internal/virtual/session.go` (`Resolve`, `Touch`),
`core/internal/audit/durable.go` (`Persist`, `persistBatch`).

## Direct low-load measurement

A fresh isolated fixture ran `TestSQLCostAcceptance` successfully in 9.21s:
one warmup, one individually measured call and twenty serial calls, no load/fault
phases. One initialized session, one server, two catalog tools, short REST
upstream, PII enabled, strict metadata audit, 4GiB Go/6Gi container/8 CPU.
All 22 calls have matching admission/outcome rows, no duplicate or incomplete
operation. Evidence: `artifacts/benchmark-runs/readiness-55-sql-cost/`.

The single-call window contained **34 top-level commands**, of which five were
identifiable background subscription claims returning no rows. The request path
therefore issued **29 commands: 8 BEGIN/COMMIT and 21 other SQL commands**.
This equals the 17-command delivery/session path plus 12 strict-audit commands
(two events, each BEGIN + SET LOCAL + three INSERTs + COMMIT). There were also
18 nested trigger/FK statements, which do not add client network exchanges.

The twenty-call interval contained 647 top-level commands and 360 nested
statements. 580 top-level commands match 20 × 29 request commands. The other 67
are background claims, cleanup, publication transactions and config revision
checking. This separation uses query identities and the inspected call path,
not subtraction of a hypothetical idle QPS. Individual pgx protocol exchanges
may batch commands; a statement count is not a TCP packet count.

The one-second idle baseline contained 41 top-level commands, including **32
empty resource-subscription claim UPDATEs**. `RunSubscriptions` starts 32 workers
per replica; each polls once per second even without subscriptions. A bounded
claim dispatcher or optional Redis wakeup hints with equivalent fallback can
reduce this fixed idle cost. At 1,000 tool calls/s this is small beside delivery
and audit work, so it is not evidence of the main throughput bottleneck.

The single-call measurement window is 0.143s **including an intentional 100ms
settling delay and statistics collection**, not tool-call latency. No capacity
conclusion is drawn. Statements remain database-scoped; shared stats were never
reset. Source/binary/resource provenance is in the acceptance report and run
`environment.json`, `images.json`, `manifest.json` and `profile.json`.

Reproduce with the `--sql-cost` command in `benchmark/kubernetes/README.md`;
`sql_cost.py` derives `sql-cost-deltas.json` from the six saved snapshots. The
archive retains snapshots, and the derived file can be regenerated offline.

## Prioritized proposals

| Priority | Candidate | Expected benefit and required evidence | Guarantees to preserve |
| --- | --- | --- | --- |
| P0 | Separate routing failures from DB cost | Diagnose connection refusals and retain all failure outcomes. Attribute the 169 MCP errors with bounded error-class counters. | No automatic POST retry of uncertain tool executions |
| P1 | Compare the same fixture on production-like PostgreSQL storage | A short low-concurrency trace and small concurrency sweep can compare commit/flush latency, pool wait and per-call WAL before another capacity test. Keep exact schema and durability settings. | fsync, synchronous commit, full-page writes, audit acknowledgement ordering |
| P1 | Reduce sequential SQL exchanges inside existing transactions | Evaluate a bounded pgx batch or a carefully reviewed database function for Begin/Append. Fewer network exchanges could matter more with a remote DB. Current local sub-ms execution means alone do not predict the gain. | Capacity checked before insert; identical transaction/fence/lease checks and rollback behavior |
| P1 | Combine the two event aggregates in Append | Count and total bytes currently scan the same stream twice. One aggregate can compute both. Impact is likely small for a single terminal event, larger for progress streams; verify EXPLAIN and event limits first. | Same operation row lock, encrypted byte accounting, reserved terminal slot and size bounds |
| P1 | Review admission budget concurrency before changing locks | A per-server advisory lock serializes a hot server; global/principal counts also cover other servers. Different-server concurrent admissions warrant a targeted boundary test to establish whether global/principal caps can overshoot. This is a code-review risk, not a failure measured by this run. | Strict global/principal/server limits; deterministic lock ordering; no lease resurrection |
| P2 | Reduce empty subscription claim polling | Measured 32 empty claims/s/replica. Evaluate one bounded dispatcher with the same 32 active workers and <=1s discovery fallback; measure wakeup/reclaim behavior. | Lease ownership, startup/reconnect discovery, cancellation and revocation checks |
| P2 | Optional Redis delivery wakeup hints | Wake the owning replica after durable PostgreSQL commit; retain bounded fallback polling. Most useful for cross-replica streams and server requests, less for this short-call same-replica path. | PostgreSQL remains authoritative for payloads, fences, leases, replay and revocation |
| P2 | Inspect audit trigger/index cost per event | Existing batching is valuable. Use per-event CPU/WAL and representative EXPLAIN before reducing indexes or rewriting validations. | Tenant provenance, immutable audit ownership, FK integrity, duplicate-ID semantics |

No evidence justifies increasing the pool or adding indexes indiscriminately.
A larger pool can deepen WAL contention. Existing partial running-operation,
expiry, request-uniqueness, child FK and `(stream_id, seq)` replay indexes already
cover important paths. The active-budget aggregate scans the currently running
set; the scan becomes more relevant with long operations. Inspect representative
cardinality and buffer counts before splitting it into multiple queries.

## Redis as an operator-selected option

The product already supports shared Redis rate limiting via `VMCP_REDIS_URL` and
an explicit failure policy. That does not remove PostgreSQL delivery writes.
The process-local circuit breaker also does not issue PostgreSQL queries, so
moving it to Redis would add coordination rather than offload database work.

The safest first additional option is **delivery notification only**, conceptually
`poll` versus `redis-hints` (proposed names, not implemented). Publish only after
PostgreSQL commit; use tenant-scoped opaque identifiers, no payloads/credentials;
authorize reads in PostgreSQL as today. Reconnect and lost messages must recover
through existing bounded polling, without extending authorization freshness or
lease intervals. Validate dropped/duplicated/reordered hints and Redis outage.

Redis Pub/Sub can lose messages. Keeping the existing fallback poll cadence
protects behavior but limits the amount of idle polling it can remove. A material
poll reduction needs a separately specified latency bound and failure-detection
proof; do not promise large throughput gains from hints alone.

Do not move authoritative session revocation, operation ownership/fencing,
durable replay or strict audit into a Redis cache. Such a backend would require a
new persistence, failover, atomicity and recovery contract. Similarly, Redis quota
counters cannot be split from PostgreSQL admission with an unsafe dual write.
An advisory prefilter may reject early, but PostgreSQL must still enforce the cap.

## Remaining low-load follow-up protocol

1. The serial short-REST baseline above is complete. For the remaining MCP/SSE/
   payload cases, initialize one session and warm metadata; record a
   no-request background baseline without resetting shared statistics.
2. Record pg_stat_statements deltas with top-level identity retained; make exactly
   one deterministic tool call with a unique request ID. Repeat serially for 10–20
   calls. Count SQL commands, transactions, audit events and WAL separately.
3. Capture representative EXPLAIN (ANALYZE, BUFFERS, WAL) only in disposable data,
   for short/long stream sizes and near-limit active operations. Mutating plans
   execute writes: use disposable transactions and never existing installations.
4. Repeat for REST/MCP, progress, payload capture and PII modes; preserve identical
   authorization and audit assertions. Add modest concurrency only after the
   single-call path is understood.
5. Compare a changed query or Redis-hint prototype against the unchanged baseline,
   including cancellation, revocation, expired-owner rejection, duplicate delivery,
   lost acknowledgement and audit reconciliation. Accept an optimization only if
   it saves measured work without changing these outcomes.

References: [PostgreSQL statistics](https://www.postgresql.org/docs/17/monitoring-stats.html),
[pg_stat_statements top/all semantics](https://www.postgresql.org/docs/17/pgstatstatements.html),
[Redis Pub/Sub delivery semantics](https://redis.io/docs/latest/develop/pubsub/).

## Follow-up: measured SQL and idle-poll reductions (2026-11-08)

The initial baseline above remains historical evidence. Follow-up source builds
start from PR head `fa098e96c48c8aeb5cec808021d2461456388d45` plus the identified
working-tree changes; full source hashes and images appear in the environment
acceptance report. Four sequential, fresh Kubernetes fixtures used the same
PostgreSQL 17 OLTP/durability profile, 8 CPU / 4GiB Go target / 6Gi container limit,
one initialized session and strict metadata audit. No existing deployment changed.

1. **Independent DB/storage comparison — not run.** Root PostgreSQL fixture storage
   is the host's ext4 NVMe filesystem. Other discovered disks contain existing NTFS
   partitions; none is verified as allocated test DB storage. No existing partition
   was mounted, formatted or repurposed. A separately authorized test DB/storage
   target remains necessary; same-disk directories do not satisfy this comparison.
2. **SQL reduction — passed functional/command-count validation.** Admission now
   creates its stream from the inserted operation using a dependent CTE, and terminal
   event insertion/completion use a dependent CTE within the existing transaction.
   A zero-row admission fails closed. The event bound query computes count and
   encrypted bytes in one aggregate scan. The operation row lock, fence, lease,
   reserved terminal budget and transaction boundaries are preserved. In the twenty
   serial calls, request commands fell from 580 to 540 (29 to 27 per call, 6.9%).
   Eight BEGIN/COMMIT commands and all durable audit writes remain per short call
   in this low-concurrency path. This does not reduce the number of commits.
3. **Empty subscription discovery — passed functional/command-count validation.**
   One dispatcher searches per replica, with 32 active worker slots and a one-second
   discovery tick. Existing worker renewal, authorization and cleanup are unchanged.
   Empty claims fell from 32 in a one-second baseline to 1 in the first changed
   one-second sample, then 5 in a five-second confirmation. This is 32 to 1 claims/s
   in these idle samples, not a claim about active subscriptions or total DB traffic.
   Completed workers are retried on a tick, avoiding immediate retry loops.

| Variant | SQL fixture duration | Request commands/call | Idle subscription claims | Median call ms (21 calls, warmup excluded) |
| --- | ---: | ---: | ---: | ---: |
| baseline | 9.24s | 29 | 32 / 1s | 21.955 |
| sql | 9.31s | 27 | 32 / 1s | 21.258 |
| dispatch | 8.65s | 27 | 1 / 1s | 21.328 |
| final | 11.84s | 27 | 5 / 5s | 21.473 |

The first changed SQL run had 608 total commands in the serial window (540 request
commands plus background work); the dispatcher run had 558 and the confirmation
had 552. Background variation must not be attributed to the request. Nested FK
checks remain 18 per call. Tiny samples on shared hardware show no established
latency improvement and provide no p95/p99, throughput, RPO or RTO acceptance.
No high-load run was launched during this follow-up.

The final fixture passed 14 selected tests with the race detector, zero failures
or skips: delivery limits, encrypted byte accounting, terminal rollback,
fencing/replay/isolation, expired workers, dispatcher capacity/cancellation,
subscription failover/disconnect/initialization cancellation, cross-node catalog,
audit maintenance and two-node revocation. The SQL fixture separately reconciled
22 admissions/outcomes with zero duplicates, failures or incomplete operations.
Python benchmark tooling: 17 tests passed. Required GitHub checks must also pass
on the final PR head before integration.

No Redis offload is introduced. It remains a separate opt-in design requiring
lost/duplicate hint, outage and tenant-isolation tests. Production-like storage,
concurrent workload sweeps and the remaining capacity/HA acceptance matrix remain
open; these low-load changes do not convert earlier failed capacity gates to passes.

## Requested 1,000 TPS repeat on df74a3b

The subsequent 600-second repeat passed the existing short-call gates: 999.267
successful calls/s, 99.945496% success, p95 173.030ms and p99 522.429ms. There were
327 failed calls and 39 dropped scheduled calls, all retained. All 600,384
successful calls including warmup matched admission/outcome records, with zero
duplicates/incomplete operations. See the environment acceptance report for
source/image digests, exact configuration, thresholds and archived evidence.

This result does not establish a causal performance improvement from the two
SQL changes: previous NodePort cooldown errors did not recur, and p99 increased
from 494.225ms. PostgreSQL WAL waits still represent 81.949% of sampled active
backend observations, with 2.482ms average WAL sync and no sampled blocking or
deadlock. Application peak CPU is 2.22 cores of 8 and peak RSS 2.25GB; pool usage
reached 64/64 but no acquisition was cancelled. Keep the independent storage
comparison open and preserve the present durability/fencing behavior.

The 327 errors cluster in seconds 303–304 around loaded revocation/config refresh
(one HTTP 409 and 326 HTTP-200 MCP errors). A follow-up should retain bounded
error-class/correlation evidence to establish their exact causes; no failed
calls were reclassified as successes. No further implementation change was made
for this repeat, and the wider workload/HA/recovery acceptance remains open.

## Interpreting more than twenty database commands per tool call

The measured 27-command serial path is substantial for a gateway, but it is not
27 independent data lookups or 27 disk flushes. Its accounting is:

| Work | Commands in the measured short serial path |
| --- | ---: |
| Session resolution | 1 |
| Delivery admission: lock, expire old slot, budget, operation+stream | 4 + BEGIN/COMMIT = 6 |
| Stream claim | 1 |
| Terminal delivery: fenced bounds and event+completion | 2 + BEGIN/COMMIT = 4 |
| Initial/terminal event reads | 2 (timing-dependent) |
| Stream release | 1 |
| Admission/outcome audit: event, access context, access domains | 6 INSERT commands + 2 SET LOCAL + 4 BEGIN/COMMIT = 12 |
| Total | 27 |

Eight commands are transaction boundaries; two enforce audit synchronous commit.
There are 17 other SQL statements. These are SQL/protocol command counts, not a
network trace. The audit bulk path batches dependent inserts and groups up to 64
events; the single-event fallback executes sequentially. Do not assume every
call gets batching or that the 27-command serial count applies under load.
FK/trigger statements are nested server work and are excluded from that count.
Four explicit transactions plus stream claim/release writes also mean several
commit points; WAL group commit means they do not imply six physical syncs/call.

The cost comes mainly from durable orchestration around a tool execution: secure
session validity, admission limits, cross-node ownership/fencing, encrypted replay,
and durable admission/outcome audit with tenant-scoped provenance. These guarantees
are valuable but their current implementation is not a theoretical SQL minimum.
A transparent stateless proxy is a different workload. The observed 1,000 TPS and
resource headroom make this a cost/latency improvement opportunity rather than
evidence that the current implementation is unusable.

Safe priorities are (1) measure per-path network waits/commit cost, (2) consolidate
compatible statements within current transaction boundaries, especially audit
context/domain persistence with identical duplicate-ID semantics, and (3) reduce
redundant delivery reads using correctly bounded wakeups. Audit admission must
remain durable before execution and outcome must retain its real completion
meaning. Combining the two logical audit events, removing authoritative session
checks, deferring strict admission audit, or moving authoritative fences/replay to
Redis solely to lower a query counter is not justified. A larger cross-component
transaction redesign would need crash-boundary and audit-reconciliation proofs.
No such product change is made by the latency diagnostic.

Upstream latency changes concurrency before it changes CPU: at 100 calls/s,
400–500ms of upstream service implies about 40–50 concurrent upstream waits under
stable throughput. Existing code releases DB transactions before the network call;
it does not reserve one database connection for that entire wait. The stream still
polls events every 250ms, so delayed calls can add event SELECTs even below the
one-second renewal interval. Above one second, ownership renewal, session/auth
revalidation and stream lease checks add work. Longer-tail/multi-second/SSE
workloads therefore cannot be inferred from a 500ms measurement.

The distributed test uses 64 keys and 300 virtual servers. A single principal is
subject to 16 active operations, a server to 128 and the global budget to 1,024.
At 500ms, a single-principal 100 TPS target demands roughly 50 concurrent operations
and can encounter intentional admission limits. These protection limits must not
be relaxed merely to reproduce the distributed test throughput.

## Delayed-upstream measurement at 100 TPS

The new observational diagnostic completed 37,000 calls with zero error/drop,
zero duplicate/incomplete operation and exact audit reconciliation. Its 5/400/500ms
upstream stages measured p95 22.77/441.22/543.57ms, application median CPU
0.349/0.369/0.380 cores and sampled acquired pool maxima 2/5/5 (pool size 64).
Peak upstream waiters were 3/48/60. This supports the code finding that slow
upstreams hold logical operations/streams, not a database connection per wait.
It does not show a CPU or connection-pool bottleneck at this offered load.

Event SELECTs rose 2 → 3 → 4 per call. Total top-level commands including background
were 27.162 → 26.822 → 27.087 per successful call, because audit batching reduced
INSERT command frequency while retaining all logical events. The 27-command
serial count therefore describes a path, not an immutable amount of work under
all concurrency levels. Reducing command count is useful, but command count alone
is an inadequate performance objective; measure duration, commits, bytes, pool
wait and correctness together.

The final 5ms stage returned to 2 event reads/call and low concurrency but p95 was
48.35ms rather than the initial 22.77ms. WAL sync means rose from 0.889ms initially
to 2.690ms at 500ms upstream and stayed 2.687ms in recovery. The ordered run is
confounded by changing storage timing; no claim of complete latency recovery or
pure gateway overhead is made. Full measurements, provenance, reproduction and
scope are in `READINESS_ENVIRONMENT_ACCEPTANCE.md`.

Current assessment: keep the robust path; the measured cost is acceptable at the
operator's stated throughput, while still worth reducing deliberately. Prioritize
batching/consolidation and bounded event notification only when measured benefit
justifies complexity. Do not redesign fencing/audit or cache authorization to
chase a lower SQL counter. Remote database RTT and multi-second upstream tails are
more useful next variables than another speculative heartbeat/lease optimization.
