# FAQ and troubleshooting

## Does vMCP support stdio servers?
No. Upstreams must be Streamable HTTP or legacy HTTP+SSE. Wrap a stdio server with a bridge if needed.

## Resources and prompts?
Not yet: vMCP proxies **tools** only.

## Do I need Redis?
No. Redis only makes rate limits global across nodes ([Rate limits](Rate-Limits.md)). Everything else uses PostgreSQL.

## A tool I selected does not show up
Check, in order: the upstream is enabled and healthy; the tool still exists after a refresh (orphaned selections are not
served); the [policy](Tool-Policies.md) does not block it (`read_only` needs `readOnlyHint` from the upstream); the exposed
name is unique.

## 401/403 from the data plane
The virtual server requires an agent key (`authRequired`), the key does not list that virtual server, it expired or was
revoked, or your IP is outside `ipAllowlist` (behind a proxy, set `VMCP_TRUSTED_PROXIES`).

## 429 Too Many Requests
A [rate limit](Rate-Limits.md) was hit; honour `Retry-After`. Without Redis each node counts separately.

## Upstream calls fail with "circuit open"
The [circuit breaker](Resilience-and-HA.md) saw repeated transport failures. It probes again after the cooldown; check
the upstream's health in the UI or `GET /api/v1/upstreams`.

## TLS errors
See the table in [Upstream TLS](Upstream-TLS.md).

## The data plane did not pick up my change
It reloads via PostgreSQL `LISTEN/NOTIFY`. Check data-plane logs and that it uses the same database as the control plane.

## Stored secrets stopped working
`VMCP_MASTER_KEY` differs from the key that encrypted them (both services must share it).

## Known limitations

- The data plane's `GET` event stream returns 405: server-initiated notifications such as progress and cancellation are
  not passed through to agents.
- Legacy SSE upstreams are tested at the client level but less end-to-end than Streamable HTTP.
- Existing upstream sessions do not move to a fallback URL; only new sessions do.
- Circuit-breaker state is per node; there are no Prometheus/OpenTelemetry metrics yet.
- Connection test in the UI only uses headers/keys typed in the current session.
- SSO sessions cannot be revoked early.
- Single tenant; no Helm chart yet.
