# FAQ and troubleshooting

## Which upstream transports does vMCP support?
Only HTTP endpoints: Streamable HTTP and legacy HTTP+SSE. vMCP does not launch upstream server processes.

## Resources and prompts?
Curated resources, templates and prompts are supported with explicit selections, aliases, namespace validation, completion and subscription grants. See [catalog contract](MCP_CATALOGS.md).

## Do I need Redis?
No. Redis only makes rate limits global across nodes ([Rate limits](Rate-Limits.md)). Everything else uses PostgreSQL.

## A tool I selected does not show up
Check, in order: the upstream is enabled and healthy; the tool still exists after a refresh (orphaned selections are not
served); the [policy](Tool-Policies.md) does not block it (`read_only` needs `readOnlyHint` from the upstream); the exposed
name is unique.

## 401/403 from the data plane
- For **agent keys**: the virtual server requires auth (`authRequired`), the key does not list that virtual server, it expired or was revoked, or your IP is outside `ipAllowlist` (behind a proxy, set `VMCP_TRUSTED_PROXIES`).
- For **OAuth JWTs**: check `WWW-Authenticate` header. 401 indicates a missing or expired/invalid token (must be an asymmetric JWT with `aud` matching `VMCP_AGENT_OIDC_AUDIENCE`). 403 indicates no identity binding matched the token's claims, or the matching binding does not authorize this virtual server or client IP.

## 429 Too Many Requests
A [rate limit](Rate-Limits.md) was hit; honour `Retry-After`. Without Redis each node counts separately.

## Upstream calls fail with "circuit open"
The [circuit breaker](Resilience-and-HA.md) saw repeated transport failures. It probes again after the cooldown; check
the upstream's health in the UI or `GET /api/v1/upstreams`. Note that OAuth token acquisition failures (IdP down or invalid client credentials) do not count toward circuit breaker failure thresholds.

## TLS errors
See the table in [Upstream TLS](Upstream-TLS.md).

## The data plane did not pick up my change
It reloads via PostgreSQL `LISTEN/NOTIFY`. Check data-plane logs and that it uses the same database as the control plane.

## Stored secrets stopped working
`VMCP_MASTER_KEY` differs from the key that encrypted them (both services must share it).

## Known limitations

- Protocol baseline is 2025-06-18; newer revisions and March batching are not advertised.
- Resource update hints can be missed during physical watcher reconnection; durable gateway replay does not make upstream events exactly once.
- Unsafe calls with ambiguous outcomes are not automatically repeated.
- Circuit-breaker state and local fallback rate limits are per node; shared strict rate limits require Redis.
- Regex masking does not guarantee privacy or prompt-injection prevention. Binary content is denied when inspection is required.
- JWT early revocation uses exact-token digests; arbitrary opaque tokens/introspection are unsupported.
- Tenant isolation uses dedicated plane/database deployments; shared-schema tenant switching is excluded.
- Production HA, restore, workload scaling and compliance gates require operator evidence. See [deployment](PRODUCTION_DEPLOYMENT.md).
