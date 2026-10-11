# Rate limits

`rateLimitPerMinute` on an [agent key](Agent-Keys-and-Authentication.md) and on a [virtual server](Virtual-Servers.md)
caps protected tool/resource/prompt calls per minute. Both are checked; either can reject.

- Algorithm: token bucket (bursts up to the limit, refilling evenly).
- Over the limit: HTTP `429` with a `Retry-After` header, JSON-RPC error code `-32029`, and an audit event with status
  `denied` and reason `rate limit`.
- `null`/unset means unlimited. `PATCH` with `null` clears a limit.

## Per node vs global

Without Redis each data-plane node counts on its own, so with *N* nodes the effective limit is up to *N* times the
configured value. To share counters, set `VMCP_REDIS_URL` on the data plane (for example `redis://redis:6379/0`).

- Redis is **optional** and used for nothing else. Sessions, config and everything essential stay in PostgreSQL.
- `VMCP_REDIS_FAILURE_POLICY=local` (default) falls back to per-node counting on Redis failure and reports degradation.
- `VMCP_REDIS_FAILURE_POLICY=strict` denies quota checks during Redis failure and makes readiness fail. Use this when a global quota is a security boundary; the manual demo enables strict mode.
- A call rejected by the virtual-server limit may already have consumed one token of the key's limit.
