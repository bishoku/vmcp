# Core performance baseline and first optimization package

The benchmarks use synthetic tools and disposable, independently migrated databases.
They never call configured production tools or modify the application's database.
The PostgreSQL user must be able to create/drop test databases; cleanup drops only
randomly named `vmcp_t_*` databases created by the test helper.

## Reproduce

Set `VMCP_TEST_DATABASE_URL` to your local test PostgreSQL connection string, then:

```sh
./scripts/benchmark-core.sh > /tmp/vmcp-bench.txt
```

`VMCP_BENCH_TIME` defaults to `1s`; `VMCP_BENCH_COUNT` defaults to `3`.
Run comparisons on the same machine with the same Go/PostgreSQL versions, process
settings and background load. Use more repetitions and longer runs for deployment
sizing. Race-instrumented timings are correctness checks, not performance results.

The suite covers:

- Catalog routing and masked metadata: 100 and 10,000 resources/tools, fallback
  versus prepared snapshot data. Lookup targets the last item to expose linear
  scan cost. `metadata` measures preparing/reusing the list, **not HTTP JSON
  serialization or transferring 10,000 definitions**. `tool-page` lists up to 200.
- Snapshot preparation: time, total allocated bytes and approximate additional
  live heap after GC for 10,000 tools plus 10,000 resources with email masking.
- Audit: 8/64 new events per durable transaction, sequential reference versus
  pipelined inserts. Both force `synchronous_commit=on`; event generation is outside
  the timer. The sequential reference matches the previous insert/commit path.
- Metrics contention: 2,000 series and continuous scrapes alongside parallel
  operation updates. This intentionally stresses the scrape lock; allocations are
  process-wide and include the concurrent scraper.
- Persistent-session calls: authenticated `alpha__add`, real PostgreSQL session
  validation, durable admission/outcome audit and HTTP/operation instrumentation.
  Background inventories add 1 or 100 upstreams and virtual servers with 100 tool
  selections each. Active calls use the same synthetic fixture tool so inventory
  growth is measured independently from upstream latency. At most eight calls are
  in flight. Sessions/connections are warmed before timing; initialization is not
  included. Calls are not retried to hide errors or 429s.

The call benchmark reports p95/p99 in milliseconds from the first 10,000 service
latency samples, excluding the local eight-slot queue. `ns/op` is aggregate wall
throughput cost, not individual call latency. `sql/call` counts traced query and
batch statements, including transaction and background snapshot work, not network
round trips. `db-acquires/call` counts pool acquisitions. These fixtures are a
repeatable baseline, **not a sustained multi-node capacity claim**.

For CPU/allocation/lock profiles, select one package and benchmark:

```sh
cd core
go test -run '^$' -bench BenchmarkOperationsWithScrapes -benchtime=10s \
  -mutexprofile=/tmp/vmcp-mutex.out -blockprofile=/tmp/vmcp-block.out ./internal/telemetry
go test -run '^$' -bench BenchmarkPrepareSnapshotMetadata -benchtime=3s \
  -cpuprofile=/tmp/vmcp-cpu.out -memprofile=/tmp/vmcp-memory.out ./internal/config
```

## Behavior retained

Metrics copy counters/maps/buckets under the registry lock, then sort and format
outside it. A blocked HTTP writer cannot hold the request/operation lock. Names,
labels, cardinality limits, histogram buckets and outcomes are unchanged.

Audit batching still performs admission before upstream side effects and only
acknowledges after a durable transaction commit. Pipelining changes wire trips,
not the number of logical audit records. Statement order, nullable fields,
event-ID idempotency, lifecycle uniqueness and individual fallback after a failed
batch remain intact. No async acknowledgment, lease or relaxed commit mode was
introduced.

Prepared metadata belongs to one immutable snapshot. It is rebuilt after policy
changes; arguments/results continue to be masked per call. Exact catalog routing
keeps the first configured match; compiled URI templates retain configuration
order. Tool errors remain scoped to their page. Catalog metadata errors remain
whole-kind errors before pagination. Empty lists, extension metadata, exposed
names, capability flags and revision-bound cursors retain their original behavior.
Unprepared fixtures retain a fallback path for differential tests and benchmarks.

## Initial local results

See `results/first-package.txt` for measured samples and the summary added with
this implementation. The comparison uses pre-package source revision `c12a4e5`
in a separate temporary source tree with the identical benchmark harness.
The baseline changes only test helper signatures to accept `testing.TB`.

Preparation moves CPU/allocations from repeated requests to configuration loads.
The initial masking fixture allocates about 72 MB during preparation, retains about
11.3 MB additionally after GC, and takes about 80 ms for 10,000 tools plus 10,000
resources. This is additional to the raw snapshot and is a real tradeoff: track
snapshot refresh duration, authorization freshness and heap at larger inventories.
No global cache retains old revisions.

## Correctness gates

Run `go test -race -count=1 ./...` in `core` with the test PostgreSQL URL and, when
available, `VMCP_TEST_REDIS_URL`, `VMCP_TEST_KAFKA_BROKERS`,
`VMCP_TEST_RABBITMQ_URL` and `VMCP_REQUIRE_SDK_INTEROP=1`.
`VMCP_REQUIRE_INTEGRATION=1` requires all external services, not just PostgreSQL.
Broker fixtures are defined in `tests/audit-exporters/compose.yaml`.

The gates include admission crash persistence, audit rollback/idempotency/fallback,
policy and revocation consistency, session lifecycle and cross-node recovery,
catalog pagination/extension fields/PII, cancellation, TypeScript SDK interop and
concurrent metrics tests. Differential tests compare complete list response bodies
for prepared/fallback snapshots, including stale cursors and invalid metadata.


## Per-call header and caller metadata cost (2026-10-21)

`internal/callercontext/performance_test.go` measures the added request path:
field selection, caller metadata merge, context attachment and HTTP header copies.
It excludes HTTP parsing/serialization, network, TLS, database, audit capture and
upstream token verification. The reference called `before` omits this path; it is
not a checkout of an earlier gateway release. `disabled` executes today's path
with both features unconfigured.

Run from `core`:

```sh
go test ./internal/callercontext -run '^$' -bench BenchmarkForwarding \
  -benchmem -benchtime=300ms -count=3 -cpu=8
go test ./internal/callercontext -run '^$' -bench BenchmarkUnknownMetadataFields \
  -benchmem -benchtime=300ms -count=3 -cpu=8
```

Local medians of three serial samples, Go 1.27.3, Xeon Silver 4216, GOMAXPROCS=8:

| Scenario | MCP time | REST time | MCP allocated bytes | REST allocated bytes |
|---|---:|---:|---:|---:|
| Features disabled | 0.292 µs | 0.301 µs | 144 | 144 |
| Two header mappings | 4.733 µs | 4.939 µs | 1,008 | 1,008 |
| Two caller metadata fields | 31.491 µs | 37.842 µs | 16,729 | 17,583 |
| Both features | 36.862 µs | 45.760 µs | 17,962 | 18,102 |
| Large metadata and headers | 413.219 µs | 435.685 µs | 223,596 | 232,016 |

Normal fixtures contain a synthetic 2 KiB token and 36-byte session value per
channel. The large fixture contains 16 metadata fields of 1,950 bytes each and
four 8 KiB headers; metadata and mapped headers have separate 32 KiB limits.
These are allocated bytes per operation, not retained heap. Full raw samples,
including eight-worker parallel runs, are in `results/header-context.txt`.
Parallel ns/op measures aggregate throughput cost, not individual call latency.

A separate case adds 2,400 unknown tiny fields alongside the two selected fields,
remaining below the 32 KiB metadata limit. `Prepare` alone takes a median 1.496 ms
for MCP and 1.451 ms for REST, allocating about 464 KB and 4,857/4,862 objects.
Unknown fields are filtered correctly, but parsing them into maps still allocates.
A byte limit therefore does not prevent substantial allocation amplification.

The main optimization opportunities found by code inspection are:

- Skip empty maps/clones/context attachment when neither feature produces headers.
- For REST, avoid creating and merging outgoing MCP metadata that the REST
  executor does not use. Required-field and value validation must remain intact.
- For MCP, avoid serializing selected metadata only to parse and serialize it again
  in `WithCallerMeta`; preserve gateway progress-token correlation and filtering.
- Bound metadata object cardinality or selectively parse configured fields with a
  bounded parser, so unknown fields cannot create thousands of map entries.

The request path adds no database query or external token-verification request.
Values are local to the call and do not enter the upstream connection fingerprint;
rotating a caller token therefore does not reconnect an upstream session. Editing
the rules changes the snapshot fingerprint and can retire cached MCP clients.
Large forwarded values also consume network bandwidth; captured MCP metadata can
increase existing audit serialization/encryption/storage costs. Neither effect is
included in the table. End-to-end p95/p99 and sustained deployment capacity still
require representative load tests with the target audit and upstream configuration.
