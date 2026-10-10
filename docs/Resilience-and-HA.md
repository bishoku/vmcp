# Resilience and high availability

Everything here needs only PostgreSQL. Redis is not required.

## Multiple data-plane nodes

Run as many nodes as you like on the same database. Session records are shared, so a session opened on one node can
continue on another (its upstream sessions are re-established on first use). Add a load balancer; affinity on
`Mcp-Session-Id` is an optimization.

## Circuit breaker

Per node and upstream. After `VMCP_BREAKER_THRESHOLD` consecutive **transport** failures (default 5, `0` disables), calls
are refused for `VMCP_BREAKER_COOLDOWN` (default 30s), then a single probe call decides. Any reply from the upstream, even an
error, counts as the upstream being alive and never trips the breaker.

## Retries

Only for tools the upstream annotates with `readOnlyHint` or `idempotentHint`: one retry with jitter after a transport
failure. Other tools are never retried, to avoid duplicate side effects.

## Failover URLs

`fallbackUrls` are tried in order when the primary URL does not answer, for tool discovery, health checks and new agent
sessions. Limitations: an *existing* upstream session stays on the URL it opened; new sessions pick the first URL that works,
and with a black-holed primary each new session may wait up to 10 seconds before falling back.

## Health checks

Every `VMCP_HEALTH_INTERVAL` (default 30s) each node probes every enabled upstream and publishes the result.
`GET /api/v1/upstreams` shows `health.state` (`healthy`, `degraded`, `unhealthy`, `unknown`) and per-node details
(latency, breaker state, last error); the UI lists them in the upstream panel. Probes feed the breaker as well.

## What is not shared

Breaker state and local rate-limit counters are per node. That is deliberate: it keeps the core free of extra
dependencies. A Redis-shared breaker is a possible future addition.
