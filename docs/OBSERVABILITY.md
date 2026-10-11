# Operations and measured baseline (P06)

Both planes serve Prometheus text at `/metrics`. Restrict this endpoint to the
monitoring network. HTTP histogram labels are one of `api`, `mcp`, `probe` or
`other`; paths, query parameters, methods supplied by callers, tool names,
subjects and credentials are never labels. Active HTTP requests cover current
MCP calls; `streams_active` reports downstream SSE streams. Data-plane
metrics include sessions, snapshot confirmation age/revision and last refresh
latency, listener reconnects, aggregate breaker states, pool acquisition counts,
cumulative wait time, audit failures/loss/queue and durable incomplete age.
Audit incomplete queries have a one-second scrape budget. The separate HTTPS audit
exporter logs delivery-state counts each minute; Prometheus export-lag/dead-letter
metrics remain on the production roadmap. See [audit operations](AUDIT_OPERATIONS.md).

Import [the Grafana dashboard](../deploy/observability/dashboard.json) and load
[Prometheus rules](../deploy/observability/alerts.yml). Change the stale-age alert
when changing `VMCP_AUTH_MAX_STALE`. Histogram p95/p99 are estimates. Pool acquire
seconds includes successful acquisition overhead, not solely saturation delay;
compare empty/cancelled acquire counts and utilization. A failed metrics scrape
is itself an alert condition; missing audit samples do not mean zero backlog.

## Tracing

Set `VMCP_OTEL_ENABLED=true`, `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` to an operator
controlled OTLP HTTP traces endpoint, and optionally `VMCP_OTEL_SAMPLE_RATIO`
(default `0.1`, range 0–1). Official OTel SDK environment settings configure TLS
and exporter authentication. Queues are bounded to 2,048 spans, exports to 256
spans and two seconds, with a three-second shutdown allowance. Collector outages
can drop telemetry and do not block audited operations. TraceContext is accepted;
baggage is not. Sampling uses a local ratio rather than trusting an incoming
sampled flag. Spans contain service name, a fixed route class and response status;
no bodies, headers, SQL parameters or raw errors. Trace IDs correlate HTTP work
with audit events. This phase instruments HTTP spans; upstream/database child
spans remain a possible extension and are not advertised as present.

## Database and quota budgets

`VMCP_DB_MAX_CONNS=16`, `VMCP_DB_MIN_CONNS=2`, and
`VMCP_DB_ACQUIRE_TIMEOUT=2s` apply to each plane's request pool. Max must be 2–1000,
min 0–max, and acquisition timeout positive and at most 30s. Connection attempts
and startup ping have five-second budgets. Connections retire after an hour and
idle connections after five minutes. URL pool min/max options are overridden by
these explicit environment settings. The connection-acquisition deadline ends
at acquisition; it does not truncate an admitted SQL statement or a healthy SSE
stream. Existing caller deadlines still bound query execution.

Each data-plane node also opens **one dedicated direct LISTEN connection** outside
its pool. Set `VMCP_DB_LISTENER_URL` to direct PostgreSQL when the ordinary URL is
PgBouncer transaction pooling. LISTEN must use a direct connection or verified
session pooling; transaction pooling is unsupported for the listener. Budget
`DP replicas × (max pool + 2 listeners) + CP replicas × max pool`, plus migrations,
backup, monitoring and reserved administration connections. Four DP replicas and
two CP replicas at defaults allow 100 application connections. These defaults
are conservative initial settings, not measured production capacity guarantees.

Listener loss increments a counter and reconnects after two seconds; polling
continues, and stale authorization fails closed. Data-plane readiness checks
snapshot freshness, writable-primary database availability and draining state.
Strict quota mode additionally checks Redis on readiness, so recovery does not
depend on receiving requests while unready. Liveness stays dependency independent.
A database role losing INSERT permission is enforced by durable admission even
if the writable-primary readiness probe succeeds.

With Redis, `VMCP_REDIS_FAILURE_POLICY=local` (default) falls back to per-node
buckets. `strict` denies configured quotas during Redis failure and marks the
node unready. Without configured quotas, no global quota is implied. Redis is
never a session/configuration/audit source of truth. Independent local counters
can multiply allowed throughput by replica count. Automated regression tests
exercise quota outage, listener termination/reconnection, pool exhaustion and
snapshot freshness; actual PostgreSQL primary promotion/backup restore remains
part of P13 deployment testing.
