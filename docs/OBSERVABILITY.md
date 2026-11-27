# Operations and measured baseline

Control-plane, data-plane and omni-mcp serve Prometheus text at `/metrics`. Restrict this endpoint to the
monitoring network. HTTP histogram labels are one of `api`, `mcp`, `probe` or
`other`; paths, query parameters, methods supplied by callers,
subjects and credentials are never HTTP histogram labels. Separate operation
families below use configured virtual-server/upstream/tool names. Active HTTP requests cover current
MCP calls; `streams_active` reports downstream SSE streams. Data-plane
metrics include sessions, snapshot confirmation age/revision and last refresh
latency, listener reconnects, aggregate breaker states, pool acquisition counts,
cumulative wait time, audit failures/loss/queue and durable incomplete age.
Each plane samples audit backlog on startup and every 30 seconds with a one-second query timeout. Prometheus scrapes and OTLP exports read the same snapshot. The two audit backlog gauges are absent until the first success and after a failed refresh. The separate HTTPS audit
exporter logs delivery-state counts each minute; Prometheus export-lag/dead-letter
metrics remain on the production roadmap. See [audit operations](AUDIT_OPERATIONS.md).

Import [the Grafana dashboard](../deploy/observability/dashboard.json) and load
[Prometheus rules](../deploy/observability/alerts.yml). Change the stale-age alert
when changing `VMCP_AUTH_MAX_STALE`. Histogram p95/p99 are estimates. Pool acquire
seconds includes successful acquisition overhead, not solely saturation delay;
compare empty/cancelled acquire counts and utilization. A failed metrics scrape
is itself an alert condition; missing audit samples do not mean zero backlog.

## OpenTelemetry

The three services expose their existing metrics at the private `/metrics` endpoint
and write JSON logs to stdout by default. Set one environment variable on each
service to export metrics, logs and traces directly to a Collector with an
OTLP/HTTP protobuf receiver and pipelines for all three signals.

```sh
OTEL_EXPORTER_OTLP_ENDPOINT=https://collector.example:4318
```

With HTTP/protobuf (the default), the SDK appends `/v1/metrics`, `/v1/logs` and `/v1/traces` to the common endpoint.
For a single signal, configure only its full URL, e.g.
`OTEL_EXPORTER_OTLP_TRACES_ENDPOINT=https://collector.example:4318/v1/traces`.

For a Collector with an OTLP/gRPC receiver, set both variables on each service.

```sh
OTEL_EXPORTER_OTLP_PROTOCOL=grpc
OTEL_EXPORTER_OTLP_ENDPOINT=https://collector.example:4317
```

Use the receiver address for gRPC. Signal-specific HTTP paths apply only to
HTTP/protobuf. To mix transports, configure the protocol and matching endpoint
for the individual signal, for example `OTEL_EXPORTER_OTLP_LOGS_PROTOCOL` and
`OTEL_EXPORTER_OTLP_LOGS_ENDPOINT`.

Signal-specific protocols and endpoints take precedence over their common settings. A signal with no destination stays
local. `OTEL_{TRACES,LOGS,METRICS}_EXPORTER=none` disables the corresponding
remote output when a common endpoint is set. Keep one ingestion path per signal
per backend to avoid double-counting scrapes or duplicating stdout logs.

Use standard SDK settings for exporter headers, TLS certificates, timeouts,
`OTEL_METRIC_EXPORT_INTERVAL` (milliseconds), `OTEL_RESOURCE_ATTRIBUTES`
and `OTEL_SERVICE_NAME`. Service names default to `vmcp-control-plane`,
`vmcp-data-plane` and `vmcp-omni-mcp`. Send credentials through secret-backed
environment variables. Helm accepts these settings through the service environment
and requires collector egress when NetworkPolicy is enabled.

Existing deployments using `VMCP_OTEL_ENABLED=true` and a traces endpoint
continue exporting traces. Explicit `VMCP_OTEL_ENABLED=false` or
`OTEL_SDK_DISABLED=true` disables remote exports and preserves local scrape
and stdout output. Sampling defaults to the local trace-ID ratio of 0.1.
`OTEL_TRACES_SAMPLER` and `OTEL_TRACES_SAMPLER_ARG` select an SDK sampler
and take precedence over the legacy `VMCP_OTEL_SAMPLE_RATIO`.

HTTP spans report route class and response status. Trusted Omni gateway calls
propagate W3C TraceContext and create a client span. Request-scoped logs carry
trace and span IDs. Request bodies, credentials, baggage and raw errors stay
outside the instrumentation. Enable `VMCP_LOG_LEVEL=DEBUG` to see HTTP completion
logs. Bounded asynchronous log and trace batches can drop telemetry during
outages. Collector availability is independent of readiness and durable audit.
Each process allows up to three seconds for telemetry shutdown.

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
part of [deployment acceptance](PRODUCTION_DEPLOYMENT.md#availability-and-disaster-recovery-acceptance).

## Background maintenance metrics

Both planes expose `vmcp_background_operations_total{activity,outcome}` for
background maintenance. Labels are fixed and never contain identities, session
IDs, resource URIs, payloads or raw error messages. Discovery uses
`activity="discovery"` and outcomes `changed`, `unchanged`, `error`, `cancelled`;
subscription workers use `activity="subscription"` and their final `ok`, `error`,
`cancelled` outcome when they exit. Counters reset on process restart.

Routine polling, reconnects and lease maintenance use operational logs and this
counter, rather than audit lifecycles. Only meaningful discovered catalog changes
produce `upstream.catalog_changed` audit mutations. Client MCP calls and explicit
administrative operations retain their existing auditing. See [Audit log](Audit-Log.md).

## Virtual server and upstream metrics

Data planes additionally expose the following families. Each family has a
`_total` counter with `outcome`, an `_active` gauge without outcome, and a
`_duration_seconds` histogram (`_bucket`, `_count`, `_sum`) without outcome:

- `vmcp_virtual_requests`: label `virtual_server`. HTTP requests (POST, GET SSE,
  DELETE) to configured virtual server routes, including authentication failures.
  HTTP status >=400 is `error`; HTTP 200 MCP errors remain HTTP `ok` here.
  Streaming duration is the HTTP connection lifetime.
- `vmcp_virtual_tool_calls`: labels `virtual_server`, `tool`. Resolved tool
  invocations, using the exposed/aliased tool name. Includes configured blocked
  tools, policy/quota denials and upstream/result failures. `outcome` is `ok`,
  `tool_error`, `denied` or `error`; failed audit outcome persistence is an error.
  Durable SSE workers record the operation once, independent of replay clients.
- `vmcp_upstream_requests`: labels `upstream`, `method`, `tool`. Gateway tool
  attempts use the original upstream tool name. Curated resource read, prompt
  get and completion dispatch use an empty tool label, never the resource URI.
  Duration includes upstream session acquisition/setup and the response. Outcomes
  are `ok`, `tool_error`, `error` or `cancelled`. Outer resilience retries count
  separately; internal OAuth/session recovery is included within an observation,
  so these are not physical HTTP packet/request counts. Policy/quota/open-breaker rejections that never enter an upstream attempt do
  not create upstream observations. Reauthorization/setup failures inside an
  entered attempt are counted as errors.

These are per-process counters; aggregate across the two scrape instances with
`sum by (virtual_server,tool,outcome)` or `sum by (upstream,method,tool,outcome)`.
They are not unique-user counts, billing records or audit evidence. Upstream
initialization/discovery/health probes, client feature callbacks and background
subscription maintenance are not independently instrumented by these families.
Unauthorized/unknown tool names never become labels; unknown virtual routes remain
in aggregate HTTP metrics only. Resource/prompt identities, arguments, results,
credentials, user IDs and raw error text are excluded.

Configured names themselves may be sensitive metadata. Restrict `/metrics`,
Prometheus and Grafana accordingly. Each data-plane registry admits at most 2,048
operation label sets plus three family overflow sets. Additional new sets and
configured names longer than 256 bytes use `__overflow__` labels and increment
`vmcp_operation_label_overflows_total`. Do not use that reserved value in names.
Inactive sets expire after one hour (cleanup runs at most once per minute);
active observations are retained. Counter resets/series disappearance can follow
restarts and expiry. Prometheus retention still governs previously scraped names.
Metric families appear after matching work occurs; an empty tool dashboard before
traffic is expected. HTTP latency and semantic tool latency measure different work.

See [benchmark monitoring](../benchmark/README.md#monitoring) for provisioned
Prometheus/Grafana, scrape targets, dashboards, alert evaluation and a repeatable
real-stack check. Alerts are evaluated by Prometheus; no notification receiver is
configured in the local sandbox.

## Readiness hardening signals

`VMCP_LOG_LEVEL` selects DEBUG, INFO (default), WARN or ERROR at startup for the
control plane, data plane, Omni and audit exporters. Invalid values fail startup.
Context-aware logs include trace/span IDs when present. Unknown error objects are
logged by type instead of raw text; explicit safe diagnostic fields remain usable.
Do not place credentials or payloads in arbitrary string log fields.

Core `/metrics` additionally exposes `go_goroutines`, `go_memstats_heap_alloc_bytes`,
`go_memstats_alloc_bytes_total` and `go_gc_cycles_total`. Delivery renew outcomes use
`vmcp_background_operations_total{activity="delivery_renew",outcome="ok|error|cancelled"}`.
Existing pool acquire/wait and breaker-state metrics remain available. Measure
these with long-running operations and a single hot virtual server before changing
lease intervals, admission locking or authorization caching.

Each audit exporter optionally serves `/metrics` at
`VMCP_AUDIT_EXPORT_METRICS_ADDR` (for example `:9091`; disabled when unset). Expose
this listener only on a private monitoring network. It exports fixed-state receipt
counts, oldest pending age (including unclaimed events), sample age and collection
availability. Collection is bounded to five seconds and occurs once a minute;
scrapes read cached data and never query PostgreSQL. Alert on dead receipts,
growing pending age, unavailable samples and stale sample age. Broker delivery
acknowledgement remains independent of monitoring.

`vmcp_breaker_oldest_probe_seconds` shows the oldest active half-open probe
without per-upstream labels.

### Pod-local admission protection

`vmcp_delivery_admission_lock_seconds_total`, `vmcp_delivery_admission_lock_calls_total` and
`vmcp_delivery_admission_lock_errors_total` are deprecated compatibility counters that
remain zero: the DB capacity advisory lock and active-operation count have been
removed. Historical values describe older binaries. Fencing and lease metrics
remain meaningful. Monitor HTTP 429s, runtime memory/CPU and upstream waiters when
tuning the per-pod Settings limits; replica count multiplies aggregate allowance.
