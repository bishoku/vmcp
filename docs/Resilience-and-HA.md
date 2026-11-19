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

### Dual-revision MCP rollout

Deploy the dual-revision binaries to every control-plane, data-plane and Omni replica before enabling `VMCP_MCP_20260728=1`. Enable upstream negotiation on the control plane and data plane, then enable modern ingress on all public replicas before routing modern clients to them. The default retains legacy behavior. The switch covers synchronous Streamable HTTP tool workflows. Legacy SSE and REST retain their existing transports.

With modern ingress the replica handling each tool call owns its execution context. If the target requires an MCP session, that replica opens a private upstream session and closes it after the call. Independent calls can reach different replicas without sharing credentials, client capabilities or upstream application state. Legacy Virtual clients keep their existing PostgreSQL-backed logical sessions. No schema change is needed.

Drain traffic before shutdown. Abrupt replica loss can leave a legacy upstream session until its idle timeout and can leave a dispatched write with unknown outcome. Configure the load balancer to avoid replaying those POST requests. Client callbacks, subscriptions and cross-call application state require the legacy path in this release.

Each data-plane replica keeps its own five-minute upstream revision cache. The next call on an active Virtual session rechecks an expired preference using a separate discovery request. If the upstream still speaks the same revision, the replica keeps the existing session. On a revision change, new calls use the new connection and leased calls finish before the old upstream session closes. Modern synchronous invocations skip upstream logging setup on both cold and warm connections.

For a native tool configured with `retry=once`, the invocation loop owns the replay budget. An expired upstream session is retired and an unauthorized OAuth token is invalidated before a permitted retry. These recovery steps share the same limit of two tool dispatches per invocation.
