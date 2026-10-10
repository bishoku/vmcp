# Audit log

Every tool call (`tools/call`) on a virtual server is recorded, including denied and failed ones:

| Field | Meaning |
|---|---|
| `time`, `virtualServer`, `method` | When, where, which MCP method |
| `agentKey` | The authenticated key (null for anonymous) |
| `clientIp` | Source IP (honours `VMCP_TRUSTED_PROXIES`) |
| `tool`, `upstream` | Tool and the upstream that served it |
| `status` | `ok`, `tool_error` (the tool reported an error), `error` (gateway-side failure) or `denied` |
| `reason` | Why a call was denied or failed (policy, rate limit, auth, ...) |
| `durationMs`, `redactions` | Latency and number of PII redactions |

Arguments and results are **not** stored.

## Querying

`GET /api/v1/audit-events` with filters `virtualServer`, `agentKey`, `tool`, `status`, `from`/`to` (RFC 3339) and cursor
paging (see the OpenAPI document). The UI's **Audit** page offers the same filters.

## Retention

Events older than `VMCP_AUDIT_RETENTION_DAYS` (default 90; `0` keeps everything) are purged. Events are written
asynchronously in batches, so the request path never waits for the database; under extreme load the writer drops events
rather than blocking calls. Export audit data elsewhere if you need long-term or tamper-evident retention.
