# Configuration reference

All configuration is through environment variables. Durations use Go syntax (`30s`, `5m`).

## Shared

| Variable | Default | Used by | Meaning |
|---|---|---|---|
| `VMCP_DATABASE_URL` | required | both | PostgreSQL connection string |
| `VMCP_MASTER_KEY` | required | both | Base64 32-byte key (`openssl rand -base64 32`) encrypting upstream headers and client keys. **Losing it makes stored secrets unreadable; changing it invalidates them.** Both services must share it |

## Control plane

| Variable | Default | Meaning |
|---|---|---|
| `VMCP_ADMIN_TOKEN` | required | Bootstrap admin token (admin role) |
| `VMCP_CP_ADDR` | `:8081` | Listen address |
| `VMCP_AUDIT_RETENTION_DAYS` | `90` | Audit events older than this are deleted; `0` keeps forever |
| `VMCP_OIDC_ISSUER` | unset | Enables SSO when set |
| `VMCP_OIDC_CLIENT_ID` | required with issuer | OIDC client |
| `VMCP_OIDC_CLIENT_SECRET` | empty | OIDC client secret |
| `VMCP_OIDC_REDIRECT_URL` | required with issuer | Public URL of `/auth/callback` |
| `VMCP_OIDC_GROUPS_CLAIM` | `groups` | ID-token claim carrying groups |
| `VMCP_OIDC_ADMIN_GROUPS`, `VMCP_OIDC_EDITOR_GROUPS`, `VMCP_OIDC_VIEWER_GROUPS` | empty | Comma-separated groups per role |

## Data plane

| Variable | Default | Meaning |
|---|---|---|
| `VMCP_DP_ADDR` | `:8080` | Listen address |
| `VMCP_TRUSTED_PROXIES` | empty | CIDRs of reverse proxies whose `X-Forwarded-For` is trusted (for IP allowlists and audit client IPs) |
| `VMCP_BREAKER_THRESHOLD` | `5` | Consecutive transport failures that open the circuit; `0` disables |
| `VMCP_BREAKER_COOLDOWN` | `30s` | How long an open circuit refuses calls before one probe |
| `VMCP_HEALTH_INTERVAL` | `30s` | Upstream health probe interval; `0` disables |
| `VMCP_NODE_ID` | hostname | Node name shown in health reports |
| `VMCP_REDIS_URL` | unset | Optional. Shares rate-limit counters between nodes |

## Health endpoints

Both services expose `GET /healthz` (liveness) and `GET /readyz` (readiness, checks the database).

## Tests only

`VMCP_TEST_DATABASE_URL`, `VMCP_TEST_REDIS_URL`: see [Development](Development.md).
