# Configuration reference

All configuration is through environment variables. Durations use Go syntax (`30s`, `5m`).

## Shared

| Variable | Default | Used by | Meaning |
|---|---|---|---|
| `VMCP_DATABASE_URL` | required | both | PostgreSQL connection string |
| `VMCP_EGRESS_ALLOW_CIDRS` | empty | both | Comma-separated private/loopback CIDRs allowed for MCP upstreams and upstream OAuth token endpoints; metadata and prohibited destinations remain denied |
| `VMCP_EGRESS_ALLOW_HOSTS` | empty | both | Optional exact DNS names/IP literals allowed for MCP/token HTTP traffic; when empty, public unicast destinations are allowed. Host grants do not override private CIDR checks |
| `VMCP_MASTER_KEY` | required | both | Base64 32-byte key (`openssl rand -base64 32`) encrypting upstream headers and client keys. **Losing it makes stored secrets unreadable; changing it invalidates them.** Both services must share it |

## Control plane

| Variable | Default | Meaning |
|---|---|---|
| `VMCP_ADMIN_TOKEN` | required | Bootstrap admin token (admin role) |
| `VMCP_CP_ADDR` | `:8081` | Listen address |
| `VMCP_AUDIT_RETENTION_DAYS` | `90` | Audit events older than this are deleted; `0` keeps forever |
| `VMCP_OIDC_ISSUER` | unset | Enables SSO when set |
| `VMCP_OIDC_DISCOVERY_URL` | unset | Where to fetch the discovery document when it is not reachable at the issuer URL (containers, split-horizon). The issuer stays the public value tokens carry |
| `VMCP_OIDC_CLIENT_ID` | required with issuer | OIDC client |
| `VMCP_OIDC_CLIENT_SECRET` | empty | OIDC client secret |
| `VMCP_OIDC_REDIRECT_URL` | required with issuer | Public URL of `/auth/callback` |
| `VMCP_OIDC_GROUPS_CLAIM` | `groups` | ID-token claim carrying groups or roles. Nested paths work, e.g. `realm_access.roles` (Keycloak realm roles) |
| `VMCP_OIDC_ADMIN_GROUPS`, `VMCP_OIDC_EDITOR_GROUPS`, `VMCP_OIDC_VIEWER_GROUPS` | empty | Comma-separated groups per role |

## Data plane

| Variable | Default | Meaning |
|---|---|---|
| `VMCP_AUDIT_MODE` | `strict` | `strict` persists tool admission before dispatch; `best_effort` permits queue loss and does not provide complete audit coverage. Control-plane administrative audit is always strict |
| `VMCP_AUTH_MAX_STALE` | `30s` | Maximum age of successfully confirmed authorization configuration; positive Go duration. Stale MCP requests return 503 |
| `VMCP_DP_ADDR` | `:8080` | Listen address |
| `VMCP_ALLOWED_ORIGINS` | empty | Comma-separated exact browser origins for MCP POST/GET/DELETE, e.g. `https://agent.example`; no paths. Empty denies all supplied Origins. Native clients may omit Origin |
| `VMCP_TRUSTED_PROXIES` | empty | CIDRs of reverse proxies whose `X-Forwarded-For` is trusted (for IP allowlists and audit client IPs) |
| `VMCP_BREAKER_THRESHOLD` | `5` | Consecutive transport failures that open the circuit; `0` disables |
| `VMCP_BREAKER_COOLDOWN` | `30s` | How long an open circuit refuses calls before one probe |
| `VMCP_HEALTH_INTERVAL` | `30s` | Upstream health probe interval; `0` disables |
| `VMCP_NODE_ID` | hostname | Node name shown in health reports |
| `VMCP_REDIS_URL` | unset | Optional. Shares rate-limit counters between nodes |
| `VMCP_AGENT_OIDC_ISSUER` | unset | Enables OAuth for agents on the data plane when set. Expected `iss` claim |
| `VMCP_AGENT_OIDC_DISCOVERY_URL` | unset | OIDC discovery base URL if different from issuer (e.g. internal network) |
| `VMCP_AGENT_OIDC_AUDIENCE` | required with issuer | Mandatory expected `aud` claim for incoming agent JWTs |
| `VMCP_AGENT_OIDC_ALGS` | `RS256,ES256` | Allowed JWT signing algorithms (asymmetric only) |
| `VMCP_PUBLIC_URL` | unset | Canonical public base URL of the data plane (e.g. `https://gateway.example.com`). Used for RFC 9728 protected resource metadata |

## Health endpoints

Both services expose `GET /healthz` (liveness) and `GET /readyz`. Control-plane readiness checks the database. Data-plane readiness returns authorization freshness JSON and becomes 503 when confirmation expires; it does not directly probe every dependency. See [Authorization consistency](AUTHORIZATION_CONSISTENCY.md).

## Tests only

`VMCP_TEST_DATABASE_URL`, `VMCP_TEST_REDIS_URL`: see [Development](Development.md).

## MCP network policy

Set the same egress policy on both services and restart after changes. See [MCP network security](MCP_NETWORK_SECURITY.md) for defaults, private-network examples, disabled redirects/proxies, TLS behavior and deployment compatibility. MCP POST requires `Content-Type: application/json` and `Accept: application/json, text/event-stream`; GET requires `Accept: text/event-stream` even though streams currently return 405 after authentication/validation.

## Audit durability

Data-plane `/audit-status` and authenticated control-plane `/api/v1/audit-status` expose failure counters and incomplete operations. Read [Audit durability](AUDIT_DURABILITY.md) before choosing best-effort mode or rolling back migration 18.

### P05 resource and drain limits

`VMCP_DRAIN_TIMEOUT` defaults to `75s` and accepts `65s`–`2m`. On SIGTERM the
 data plane rejects new admissions and readiness returns 503, while admitted
 requests retain live snapshot and audit services. After HTTP drain (or forced
 connection closure plus a six-second outcome-write allowance), local upstream
 sessions retire, background producers stop, audit buffers flush, and pools close.
 Compose grants 90 seconds; increase that when increasing the drain timeout
 (allow at least another 15 seconds for cancellation and audit flushing).
 A killed process can leave durable incomplete admissions; P04 recovery evidence
 remains available. Shutdown does not revoke sessions stored for other nodes.

Limits are currently fixed per process: 10,000 cached sessions, 1,024 concurrent
POST requests, 128 per virtual server, and 16 per authenticated identity (all
anonymous requests share the anonymous identity bucket). These counters include
initialization and body reading and return 429 when saturated; shared sessions
and replicas do not provide cluster-wide concurrency quotas. Request bodies have
a ten-second read deadline, and JSON response writes have a ten-second deadline.
Upstream JSON responses and assembled SSE data frames are limited to 10 MiB;
discovery stops at 100 pages or 10,000 tools and rejects repeated/oversized
cursors. Health probes run at most 16 concurrently. OAuth source caching retains
at most 1,024 entries and failed token fetches back off for one second. Cache
turnover may refetch credentials; no secret is used as a telemetry label.

Non-idempotent tool calls never automatically replay a session-expiry response
or HTTP 401. A subsequent client request may reconnect/refresh; clients must
resolve uncertain side effects before retrying. Read-only/idempotent annotations
permit bounded replay and are trusted upstream declarations, not a proof of
idempotency. Caller cancellation and IdP failures do not trip upstream breakers.
