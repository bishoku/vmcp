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

Only for tools the upstream annotates with `readOnlyHint` or `idempotentHint`: one retry with jitter after a transport
failure. Other tools are never retried, to avoid duplicate side effects.

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
