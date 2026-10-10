# Rate limits

`rateLimitPerMinute` on an [agent key](Agent-Keys-and-Authentication.md) and on a [virtual server](Virtual-Servers.md)
caps tool calls per minute. Both are checked; either can reject.

- Algorithm: token bucket (bursts up to the limit, refilling evenly).
- Over the limit: HTTP `429` with a `Retry-After` header, JSON-RPC error code `-32029`, and an audit event with status
  `denied` and reason `rate limit`.
- `null`/unset means unlimited. `PATCH` with `null` clears a limit.

## Per node vs global

Without Redis each data-plane node counts on its own, so with *N* nodes the effective limit is up to *N* times the
configured value. To share counters, set `VMCP_REDIS_URL` on the data plane (for example `redis://redis:6379/0`).

- Redis is **optional** and used for nothing else. Sessions, config and everything essential stay in PostgreSQL.
- If Redis becomes unreachable, limits fall back to per-node counting and a warning is logged. Calls are never blocked
  because Redis is down (fail-open).
- A call rejected by the virtual-server limit may already have consumed one token of the key's limit.
