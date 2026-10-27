# Configuration reference

All configuration is through environment variables. Durations use Go syntax (`30s`, `5m`).

## Shared

| Variable | Default | Used by | Meaning |
|---|---|---|---|
| `VMCP_DATABASE_URL` | required | both | PostgreSQL connection string |
| `VMCP_EGRESS_ALLOW_CIDRS` | empty | both | Comma-separated private/loopback CIDRs allowed for MCP upstreams and upstream OAuth token endpoints; metadata and prohibited destinations remain denied |
| `VMCP_EGRESS_ALLOW_HOSTS` | empty | both | Optional exact DNS names/IP literals allowed for MCP/token HTTP traffic; when empty, public unicast destinations are allowed. Host grants do not override private CIDR checks |
| `VMCP_MASTER_KEY` | required | both | Base64 32-byte key (`openssl rand -base64 32`) encrypting upstream headers and client keys. Retain recovery keys; use the reader-first keyring/rotation procedure rather than replacing it blindly. Both planes in one tenant share the keyring |

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

Both services expose `GET /healthz` (liveness) and `GET /readyz`. Control-plane readiness checks the database. Data-plane readiness reports authorization freshness and fails on expired confirmation, unavailable audit writes, or a strict Redis quota dependency. Draining planes report unready. See [Authorization consistency](AUTHORIZATION_CONSISTENCY.md).

## Tests only

`VMCP_TEST_DATABASE_URL`, `VMCP_TEST_REDIS_URL`: see [Development](../maintainer/Development.md).

## MCP network policy

Set the same egress policy on both services and restart after changes. See [MCP network security](MCP_NETWORK_SECURITY.md) for defaults, private-network examples, disabled redirects/proxies, TLS behavior and deployment compatibility. MCP POST requires `Content-Type: application/json` and `Accept: application/json, text/event-stream`; GET requires `Accept: text/event-stream` and opens an authenticated, session-scoped SSE stream when durable delivery is enabled.

## Audit durability

Data-plane `/audit-status` and authenticated control-plane `/api/v1/audit-status` expose failure counters and incomplete operations. Read [Audit durability](AUDIT_DURABILITY.md) before choosing best-effort mode or rolling back migration 18.

### Resource and drain limits

`VMCP_DRAIN_TIMEOUT` defaults to `75s` and accepts `65s`–`2m`. On SIGTERM the
 data plane rejects new admissions and readiness returns 503, while admitted
 requests retain live snapshot and audit services. After HTTP drain (or forced
 connection closure plus a six-second outcome-write allowance), local upstream
 sessions retire, background producers stop, audit buffers flush, and pools close.
 Compose grants 105 seconds; increase that when increasing the drain timeout
 (allow at least another 15 seconds for cancellation and audit flushing).
 A killed process can leave durable incomplete admissions; recovery evidence
 remains available. Shutdown does not revoke sessions stored for other nodes.

Limits are currently fixed per process: 10,000 cached sessions, 1,024 concurrent
MCP admissions, 128 per virtual server, and 16 per authenticated identity (all
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


## Pool, telemetry, identity and operational services

| Variable | Default | Meaning |
|---|---|---|
| `VMCP_DB_MAX_CONNS`, `VMCP_DB_MIN_CONNS` | `16`, `2` | Request pool size per plane; account for every replica |
| `VMCP_DB_ACQUIRE_TIMEOUT` | `2s` | Pool acquisition deadline |
| `VMCP_DB_LISTENER_URL` | database URL | Direct/session-pooling PostgreSQL endpoint for persistent LISTEN; transaction pooling is unsuitable |
| `VMCP_SKIP_MIGRATIONS` | `false` | Control plane checks the schema instead of migrating; run `cmd/migrate` first |
| `VMCP_TENANT_ID` | `default` | Dedicated database deployment identity, consistent on planes and operational commands |
| `VMCP_REDIS_FAILURE_POLICY` | `local` | Per-node fallback (`local`) or quota denial/readiness failure (`strict`) on Redis outage |
| `VMCP_OTEL_ENABLED` | `false` | Enable bounded OTLP HTTP tracing |
| `VMCP_OTEL_SAMPLE_RATIO` | `0.1` | Local trace sampling ratio, 0–1 |
| `VMCP_KEY_ID` | `primary` | Primary encryption key ID |
| `VMCP_DECRYPT_KEYS` | unset | JSON recovery-key map for reader-first rotation |
| `VMCP_CIPHERTEXT_WRITE_VERSION` | `v1` | `v1` bound envelope, or transitional `legacy` during reader-first rollout |
| `VMCP_APPROVAL_URL` | unset | Optional HTTPS metadata/digest request-approval service |
| `VMCP_AUDIT_EXPORT_DATABASE_URL`, `VMCP_AUDIT_EXPORT_CONSUMER`, `VMCP_AUDIT_EXPORT_URL` | required by exporter | Separate exporter pool, stable consumer name and HTTPS sink |

See [observability](OBSERVABILITY.md) for pool/tracing constraints and OTel variables,
[security operations](SECURITY_OPERATIONS.md) for rotation/identity containment,
[audit operations](AUDIT_OPERATIONS.md) for export/retention roles, and
[tenant operations](TENANT_OPERATIONS.md) for router variables.
The [manual stack](../benchmark/README.md) supplies an explicit local configuration.

## Independent audit exporter processes

HTTPS, Kafka and RabbitMQ exporters use their own database connection pools and
configuration. See [all exporter variables and deployment examples](AUDIT_EXPORTERS.md).
No broker configuration is required by the control plane or data plane.

## Optional Omni MCP process

Omni has no database or master-key credentials. Its internal bridge, embedding
provider, refresh, rate limit, proxy and OAuth settings are listed in the
[complete Omni environment reference](Omni-MCP-Installation.md#environment-variables).

For model API compatibility, vector sizes and provider authentication (including
on-prem services without API keys), see [Omni embedding models](Omni-MCP-Embedding.md).

Optional rerank model settings and separate query-level/per-tool score policies
are documented in [Omni reranking and calibration](Omni-MCP-Reranking.md).
