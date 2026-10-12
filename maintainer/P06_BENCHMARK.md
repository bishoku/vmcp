# P06 mixed workload baseline

Measured 2026-10-11 on Apple M5 Max, 18 logical CPUs, 36 GiB host RAM, macOS,
Go 1.27.2; PostgreSQL 17 and Redis 7 in local Docker. One in-process HTTP gateway
replica, a 16-connection PostgreSQL pool plus dedicated listener; eight concurrent
anonymous clients, eight persisted sessions, 160 calls to a local fixture add
tool (no artificial latency), strict synchronous admission/outcome audit writes.
Dataset: two upstreams, one virtual server, five curated tools, 320+ audit rows.
A concurrent 20ms loop updates configuration and queries incomplete admissions.
The client explicitly retries this read-only fixture operation at most five times
during revision churn. Eight changes and seven audit queries completed; 27 client
retries occurred. All calls completed, no incomplete admissions remained.

| Measurement | Observed |
|---|---:|
| Calls p95 | 18.47 ms |
| Calls p99 | 31.77 ms |
| Session creation p95 | 7.56 ms |
| Workload wall time | 0.161 s |
| Go heap at completion | 30,573,728 bytes |
| Cumulative pool acquisition time | 0.0252 s |
| Empty-pool acquisitions | 8 |
| Last snapshot refresh | 1.712 ms |
| Pending durable outcomes / oldest age | 0 / 0 s |

Reproduce with isolated PostgreSQL and Redis test URLs:

```sh
cd core
VMCP_TEST_DATABASE_URL=postgres://... VMCP_TEST_REDIS_URL=redis://... \
  go test -count=1 -run TestOperationalLoad -v ./internal/e2e
```

The test emits a JSON `LOAD_REPORT` with measured values on every run. This short
local baseline validates mixed-workload behavior; it does not establish sustained
throughput, an enterprise SLO, multi-replica capacity, WAN/TLS overhead, large
catalog behavior, long tool execution or database failover recovery. Re-run with
representative catalog size, call duration, hardware and replica counts before
sizing production. No production scaling limit is inferred from this run.
