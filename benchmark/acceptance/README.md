# Isolated scale acceptance test

Run `./scripts/acceptance-core.sh` from the repository root. Requires Docker,
Go and the repository's existing module dependencies. The wrapper creates a unique
Compose project with its own PostgreSQL and Redis, builds the current data-plane
binary, and runs two independent native child processes. It never stops or writes
to the demo deployment. Containers and their volumes are removed on normal exit;
the child processes and random test database are cleaned up by the test harness.

The default run takes about eleven minutes, plus build/startup. Set
`VMCP_ACCEPTANCE_RACE=1` only for functional smoke/race validation, not capacity measurements. Output is saved
under `benchmark/results/acceptance-<timestamp>`, or `VMCP_ACCEPTANCE_OUTPUT`.
The wrapper renders the human report automatically after cleanup. To render it again:

```sh
python3 benchmark/acceptance/report.py /absolute/output/directory/report.json
```

The suite has an explicit opt-in; ordinary `go test ./...` skips it. Child binaries
are not race-instrumented for performance measurements. A short functional smoke
run (capacity gates disabled; result/audit consistency gates remain enabled):

```sh
VMCP_ACCEPTANCE_SMOKE=1 VMCP_ACCEPTANCE_SERVERS=8 \
VMCP_ACCEPTANCE_TOOLS=20 VMCP_ACCEPTANCE_RESOURCES=10 \
VMCP_ACCEPTANCE_RPS=20 VMCP_ACCEPTANCE_BURST_RPS=60 \
./scripts/acceptance-core.sh
```

## Default profile and gates, declared before the run

- 300 enabled logical upstreams across 16 synthetic fixture listeners; 300 virtual
  servers, each exposing 300 tools and 100 resources: 90,000 tool definitions plus
  90,000 selections, 30,000 resource definitions plus 30,000 selections.
- 600 initialized persistent sessions; 64 distinct agent keys, each granted the
  configured virtual servers. Payload/route/tool/nonce checks run on every call.
- Ten percent of virtual servers use email PII masking for metadata/results.
  Tools explicitly do **not** claim idempotency/read-only safety, preventing unsafe
  upstream replay. Fixture execution nonces detect duplicate execution.
- Two current production data-plane executables, GOMAXPROCS=2 and GOMEMLIMIT=768MiB
  each, default 64-connection pools, strict audit, Redis strict failure policy,
  default snapshot freshness and default 30-second health probing.
- Rate-limit settings remain unlimited in this baseline; Redis is provisioned but
  limiter-throughput and Redis-outage acceptance are not claimed.
- Synthetic tool response delay: 5ms. PostgreSQL: 2 CPU / 2GiB and max_connections
  200; Redis: 0.5 CPU / 256MiB. Native data-plane CPU is a Go scheduler limit, not
  an OS hard quota. The driver/fixtures share the host and are not resource-capped.

| Phase | RPS | Seconds | Fault |
|---|---:|---:|---|
| Warmup | 50 | 20 | none |
| Steady | 200 | 90 | none |
| Spike | 600 | 20 | none |
| Recovery | 200 | 45 | none |
| Node loss | 200 | 90 | kill only the owned node-1 child with SIGKILL at second 20 |
| Rejoin | 200 | 60 | restart that node before the phase |
| Soak | 100 | 300 | none |

The test-only round-robin reverse proxy applies a passive ten-second failure
cooldown and never retries a POST. Production nginx/ingress is not under test.
An in-flight unsafe call may have executed when a node dies; missing outcomes are
preserved as unknown and never synthesized as success or automatically replayed.

Steady/recovery/soak target >=95% of scheduled rate, >=99.9% success, p95
<=250ms and p99 <=1000ms. Fault phases allow >=99% aggregate success, with a
10-second transition grace followed by <=0.1% errors; passive failure probes are
included. Spike targets >=80% scheduled rate and success, p99 <10s. Per-node RSS
budget is 1.5GiB, and soak first/last-third median RSS growth must be <=20%.
These are baseline acceptance thresholds, not a universal product SLO.

## Measurement semantics

The generator is open-loop with 128 workers and no unbounded queue. Worker
exhaustion or scheduling lateness is counted as dropped demand, not hidden by
reducing the requested rate. Successful latency percentiles exclude errors; errors,
HTTP status codes, invalid payloads, node distribution and one-second counts are
reported separately. Every supplied session/upstream must receive real tool calls.

Resource samples are taken approximately every five seconds: native process RSS
and OS-reported CPU, data-plane metrics, PostgreSQL transaction/connection/lock/
deadlock/disk statistics and Docker CPU/memory. CPU percentages are OS-specific;
RSS is not Go live heap. Metrics counters reset when a node restarts. Sparse
resource sampling cannot measure instantaneous peaks or prove absence of a leak.

Successful response request IDs are matched to both durable admission and `ok`
outcome records. Incomplete admissions are joined through durable delivery records
to failed client requests; unexplained missing outcomes fail the gate. Invalid
keys, shared session deletion, propagated key revocation, cross-node session use,
pagination and metadata masking are checked separately.

Not included: real upstream service saturation, internet/network impairment,
JWT/IdP authentication load, active rate limits/Redis outages, control-plane
refresh/configuration churn under traffic, deployment ingress, PostgreSQL HA,
or multi-hour soak. Consult the observed report before sizing production capacity.

Configuration overrides: `VMCP_ACCEPTANCE_SERVERS` (1–1000),
`VMCP_ACCEPTANCE_TOOLS` (1–1000), `VMCP_ACCEPTANCE_RESOURCES` (0–1000),
`VMCP_ACCEPTANCE_RPS` (1–2000), `VMCP_ACCEPTANCE_BURST_RPS` (1–5000),
`VMCP_ACCEPTANCE_PG_PORT` and `VMCP_ACCEPTANCE_REDIS_PORT`. Keep all generated
services local; run only synthetic tools. A failing SLO returns a non-zero exit
status **after saving report.json**, with evidence retained for diagnosis.
