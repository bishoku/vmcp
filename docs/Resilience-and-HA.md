# Resilience and high availability

Everything here needs only PostgreSQL. Redis is not required.

## Multiple data-plane nodes

Run as many nodes as you like on the same database. Session records are shared, so a session opened on one node can
continue on another (its upstream sessions are re-established on first use). Add a load balancer; affinity on
`Mcp-Session-Id` is an optimization. Every shared-session lookup checks PostgreSQL, including warm caches. Successful DELETE is enforced on the next lookup across nodes; database failures return 503. Authorization snapshots expire after `VMCP_AUTH_MAX_STALE` (default 30s) without successful confirmation. See [Authorization consistency](AUTHORIZATION_CONSISTENCY.md) for ownership, rotation, in-flight behavior and upgrade requirements.

## Circuit breaker

Per node and upstream. After `VMCP_BREAKER_THRESHOLD` consecutive **transport** failures (default 5, `0` disables), calls
are refused for `VMCP_BREAKER_COOLDOWN` (default 30s), then a single probe call decides. Any reply from the upstream, even an
error, counts as the upstream being alive and never trips the breaker.

## Retries

Retries are off by default. An operator with scoped upstream invocation-security
management permission can set `retry: once` on the upstream or an individual tool.
A tool override takes precedence; `never` disables an inherited retry. This permits
one retry with 50–250 ms jitter after a transport failure, within the call deadline.
Upstream `readOnlyHint` / `idempotentHint` annotations do not grant replay permission.
REST calls and calls bound to approval or caller-context verification are never
replayed automatically, even when `once` is configured. Review ambiguous outcomes
before manually repeating a write. See [Invocation security](Invocation-Security.md).

Ignored half-open probes release their slot immediately. Completion is tied to the
probe generation so a late result cannot change a replacement probe's state.

Critical supervised workers fail readiness and initiate graceful shutdown on panic.
The gateway does not resume potentially damaged worker state or log panic values.
Pending operations remain unknown until audit/replay reconciliation; no successful
outcome is fabricated. This does not recover OOM, SIGKILL or runtime fatal errors.

## Failover URLs

`fallbackUrls` are tried in order when the primary URL does not answer, for tool discovery, health checks and new agent
sessions. Configuration changes retire cached upstream clients, so an existing agent session uses the new connection settings on its next call. Without a configuration change, an upstream session stays on the URL it opened; newly established upstream sessions pick the first URL that works,
and with a black-holed primary each new session may wait up to 10 seconds before falling back.

## Health checks

Every `VMCP_HEALTH_INTERVAL` (default 30s) each node probes every enabled upstream and publishes the result.
`GET /api/v1/upstreams` shows `health.state` (`healthy`, `degraded`, `unhealthy`, `unknown`) and per-node details
(latency, breaker state, last error); the UI lists them in the upstream panel. Probes feed the breaker as well.

## What is not shared

Breaker state and local rate-limit counters are per node. That is deliberate: it keeps the core free of extra
dependencies. A Redis-shared breaker is a possible future addition.
