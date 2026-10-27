# Install Omni MCP without Helm

Omni MCP is an optional third process, alongside the control plane and data plane.
Use matching **v1.2.2** binaries/images for all three. The new image is
`barishoku/vmcp-omni-mcp:v1.2.2` (linux/amd64 and linux/arm64); pin the release
manifest's digest in production. Existing vMCP endpoints continue to work.
See [Omni behavior and security](Omni-MCP.md) before publishing tools.

## Upgrade an existing installation

1. Back up PostgreSQL and retain the existing master encryption key.
2. Apply migrations using the v1.2.2 `migrate` binary/image and the migration DB
   credential. Migration 34 adds `omni_enabled DEFAULT false`; migration 33 adds
   teams/projects if upgrading from before v1.1.0. Never run a second migration
   job concurrently. Existing control-plane automatic migration is also supported.
3. Upgrade control plane, data plane and web to v1.2.2. Set
   `VMCP_SKIP_MIGRATIONS=true` on the control plane if a separate migration job
   applied the schema. Keep existing DB, master key, OAuth, Redis and audit settings.
4. Generate a new internal Omni secret (`openssl rand -hex 32`) and distribute it
   to all three services using your secret manager. Restart them with the settings
   below, then start Omni. This is not an agent API key.
5. Verify readiness, publish a selected vMCP, and grant an agent access through
   the existing runtime key or OAuth identity binding. Publication alone grants
   no access. Existing vMCPs remain unpublished until explicitly enabled.

To disable Omni, remove its public route and stop the process; normal endpoints
remain available. Remove the bridge secret from CP/DP and restart to disable the
private interfaces. Do not drop team/project or publication columns as a routine
rollback; retain the upgraded schema and backup.

## Environment variables

| Variable | Service | Required / default |
|---|---|---|
| `VMCP_OMNI_ALLOW_ANONYMOUS` | DP, Omni | `false`; opt in on both services to allow public access to published anonymous vMCPs |
| `VMCP_OMNI_SECRET` | CP, DP, Omni | Same random secret, at least 32 characters; required when enabled |
| `VMCP_OMNI_DATA_PLANE_URL` | Omni | Required internal DP base URL, e.g. `http://data-plane:8080`; no MCP suffix |
| `VMCP_OMNI_URL` | CP | Internal Omni base URL, e.g. `http://omni-mcp:8090`; enables management status/preview |
| `VMCP_OMNI_PUBLIC_URL` | CP, Omni | Public base URL, e.g. `https://mcp.example.com`; no MCP suffix |
| `VMCP_LISTEN` | Omni | `:8090`; bind `127.0.0.1:8090` for same-host proxy deployments |
| `VMCP_OMNI_REFRESH_INTERVAL` | Omni | `30s`, minimum `1s`; asynchronous catalog refresh |
| `VMCP_OMNI_MIN_SIMILARITY` | Omni | `0` disables the optional cosine floor; calibrate 0–1 per model on positives and out-of-catalog intents (quality update) |
| `VMCP_OMNI_SEARCH_LIMIT` | Omni | `120` searches per credential per minute **per replica**, positive integer |
| `VMCP_OMNI_EMBEDDING_URL` | Omni | Optional complete OpenAI-compatible embeddings URL, including `/v1/embeddings` |
| `VMCP_OMNI_EMBEDDING_TEXT_FORMAT` | Omni | `legacy-v1`; opt-in `tool-first-v1` bounded capability text (quality update) |
| `VMCP_OMNI_EMBEDDING_DIMENSIONS` | Omni | `0` learns size; 1–4096 validates provider output, does not select/reduce output dimensions (since v1.2.2) |
| `VMCP_OMNI_EMBEDDING_REVISION` | Omni | Optional operator-declared model revision; bump when changing weights behind an alias |
| `VMCP_OMNI_EMBEDDING_QUERY_PREFIX` | Omni | Empty; model-specific query prefix, including any trailing space |
| `VMCP_OMNI_EMBEDDING_DOCUMENT_PREFIX` | Omni | Empty; model-specific indexing prefix, including any trailing space |
| `VMCP_OMNI_EMBEDDING_MODEL` | Omni | Required when an embedding URL is set |
| `VMCP_OMNI_EMBEDDING_INSECURE_SKIP_VERIFY` | Omni | `false`; set `true` to skip TLS certificate-chain and hostname verification for embedding requests only |
| `VMCP_OMNI_EMBEDDING_KEY` | Omni | Optional provider bearer credential; unset/empty sends no Authorization header (on-prem APIs without keys are supported) |
| `VMCP_AGENT_OIDC_ISSUER` | Omni | Same issuer as DP when using OAuth; advertises resource metadata |
| `VMCP_TRUSTED_PROXIES` | Omni | Empty by default; comma-separated CIDRs of actual trusted ingress peers only |
| `GOMEMLIMIT` | Omni | Optional Go soft memory target; independently size from DP and container limit |
| `GOMAXPROCS` | Omni | Optional Go CPU concurrency setting |

Omni does **not** need `VMCP_DATABASE_URL`, `VMCP_MASTER_KEY`, an admin token or
Redis credentials. Existing CP/DP environment variables retain their meaning.
DP remains responsible for issuer/audience validation, IP rules, ownership
approvals, quotas, audit and upstream credentials. Do not set a provider API key
unless that provider is approved to receive tool descriptions and search queries.
Without an embedding URL the service explicitly operates in `keyword` mode.

### Embedding model requirements and choices

See [Embedding models for Omni MCP](Omni-MCP-Embedding.md) for the exact API
contract, supported vector sizes (**1–4,096**), fixed batch/timeout limits,
keyless on-prem and bearer-protected examples, Helm/Compose configuration, memory
sizing, compatibility checks and safe model/dimension changes. API keys are
optional; with an unset/empty provider key, Omni sends no Authorization header.

### Self-signed embedding endpoints

For an embedding service with a self-signed certificate, set this on the Omni
process and restart/recreate it:

```sh
VMCP_OMNI_EMBEDDING_URL=https://embeddings.internal/v1/embeddings
VMCP_OMNI_EMBEDDING_MODEL=your-model
VMCP_OMNI_EMBEDDING_INSECURE_SKIP_VERIFY=true
```

In a shell, export these variables; in Docker, put them in `omni.env`. The Compose
overlay passes the flag through. TLS remains encrypted, but the server certificate
and hostname are not verified. This applies only to the embedding client, not
Omni’s data-plane connection or upstream settings. The default is `false`; invalid
boolean values stop startup. Plain `http://` embedding URLs are already supported
and do not need this flag. This option is available from **v1.2.1**; v1.2.0 does not support it.

### Docker on an existing private network

The following assumes CP/DP already run on `vmcp-internal`, DP is reachable as
`data-plane:8080`, and their environments were updated as above. Create an
operator-only file; Docker `--env-file` expects literal values, not shell expansion:

```sh
umask 077
cat > omni.env <<'ENV'
VMCP_LISTEN=:8090
VMCP_OMNI_DATA_PLANE_URL=http://data-plane:8080
VMCP_OMNI_PUBLIC_URL=https://mcp.example.com
VMCP_OMNI_REFRESH_INTERVAL=30s
VMCP_OMNI_SEARCH_LIMIT=120
GOMEMLIMIT=512MiB
GOMAXPROCS=2
ENV
# Append the SAME secret provisioned in CP and DP, using your secret manager.
# VMCP_OMNI_SECRET=<random-secret-at-least-32-characters>
# Optional semantic search:
# VMCP_OMNI_EMBEDDING_URL=https://embeddings.example.com/v1/embeddings
# VMCP_OMNI_EMBEDDING_MODEL=<provider-model-name>
# VMCP_OMNI_EMBEDDING_KEY=<provider-key-if-required>

docker run -d --name vmcp-omni --restart unless-stopped \
  --network vmcp-internal --network-alias omni-mcp \
  --env-file ./omni.env --read-only --cap-drop ALL \
  --security-opt no-new-privileges --memory 1g --cpus 2 \
  -p 127.0.0.1:8090:8090 \
  barishoku/vmcp-omni-mcp:v1.2.2
curl --fail http://127.0.0.1:8090/readyz
```

The 512MiB soft / 1GiB container settings are a starting point for small catalogs;
monitor actual usage and embedding dimensions. They are independent of the
previously sized DP 4GiB soft / 6GiB container settings. The index is in memory and
rebuilt on restart; no volume is required. Restart rebuild can incur embedding
provider requests. Each replica has its own index and search quota.

### Local Docker Compose

The repository overlay builds the current source. Checkout the v1.2.2 tag to
reproduce the release; these development Compose files are not a hardened
production database/proxy installation.

```sh
export VMCP_OMNI_SECRET="$(openssl rand -hex 32)"
export VMCP_MASTER_KEY="$(openssl rand -base64 32)"
export VMCP_ADMIN_TOKEN="$(openssl rand -hex 32)"
export VMCP_OMNI_PUBLIC_URL=http://localhost:8090
# Optionally export the embedding URL/model/key listed above.
docker compose -f deploy/docker-compose.yml -f deploy/docker-compose.omni.yml up -d --build
curl --fail http://127.0.0.1:8090/readyz
```

Persist these secrets securely before recreating the installation; never generate
another master key for an existing DB. The overlay publishes Omni on loopback
only. Existing upstream network allowlists still apply. To increase the search
quota in Compose, the overlay passes `VMCP_OMNI_SEARCH_LIMIT` through explicitly.

### Native binary / process supervisor

The `release-artifacts` GitHub Actions run produces `vmcp-omni-mcp-linux-amd64`
and `vmcp-omni-mcp-linux-arm64` with checksums in its platform artifact. These
binaries are separate from the chart/image assets on the GitHub Release. Download
the appropriate workflow artifact and verify its checksum, or build from source:

```sh
cd core
go build -trimpath -o ./bin/omni-mcp ./cmd/omni-mcp
```

Run under a dedicated unprivileged account through your supervisor. Supply the
same environment table, using `VMCP_LISTEN=127.0.0.1:8090` and
`VMCP_OMNI_DATA_PLANE_URL=http://127.0.0.1:8080` for same-host processes. A systemd
service can use `EnvironmentFile=/etc/vmcp/omni.env`,
`ExecStart=/opt/vmcp/omni-mcp`, `Restart=on-failure`, `NoNewPrivileges=true`,
`ProtectSystem=strict`, `PrivateTmp=true`, and `MemoryMax=1G`. Keep the environment
file readable only by the operator/service account. Allow at least 75 seconds
for graceful shutdown of outstanding requests.

## Public routing and OAuth

Terminate TLS at the reverse proxy. Expose **only**:

- `/mcp/v1/omni` → Omni
- `/.well-known/oauth-protected-resource/mcp/v1/omni` → Omni (for OAuth)

Keep `/internal/*`, `/metrics`, `/healthz`, `/readyz` private. In particular,
adding a broad `location /` proxy to Omni would expose operational routes.
For nginx on the same host, add these exact locations to an existing TLS server:

```nginx
location = /mcp/v1/omni {
    proxy_pass http://127.0.0.1:8090;
    proxy_set_header Authorization $http_authorization;
    proxy_set_header X-Forwarded-For $remote_addr;
    proxy_set_header X-VMCP-Omni-Secret "";
    proxy_set_header X-VMCP-Client-IP "";
    proxy_read_timeout 75s;
    proxy_buffering off;
}
location = /.well-known/oauth-protected-resource/mcp/v1/omni {
    proxy_pass http://127.0.0.1:8090;
}
```

Trust only the nginx peer address in Omni's `VMCP_TRUSTED_PROXIES`. For a Docker
proxy use the actual proxy network/peer rather than assuming loopback. Preserve
existing routes for CP, direct vMCP and OAuth. CP/DP private bridge ports must also
be restricted by the existing proxy/firewall.

Omni and DP form **one logical OAuth protected resource**. Configure the IdP so
the issued token has the audience expected by DP; set the same issuer on Omni.
An independent Omni audience/token-exchange flow is not implemented. Clients use
existing runtime keys or OAuth access tokens, never the management login token or
internal Omni secret. The browser preview requires an explicit runtime token.

## Verify installation

1. Check private `/healthz` (process alive), `/readyz` (initial catalog available),
   and `/metrics`. Ready does not guarantee embedding quality or DP availability;
   caller operations independently fail closed if DP authorization is unavailable.
2. In the vMCP editor enable **Publish through Omni MCP**. In **Omni MCP**, inspect
   the endpoint/index and optionally request refresh. Admins see global index data;
   other users see only their authorized discovery results.
3. Connect an MCP client to `https://mcp.example.com/mcp/v1/omni` using its runtime
   credential. `tools/list` must return exactly the four Omni tools. Run
   `discover_tools`, then `get_tool_schema`, then `invoke_tool` with the returned
   `tool_ref` and `schema_version`. See [protocol examples](Omni-MCP.md).
4. Check the original agent identity, target vMCP/tool and trace in runtime audit.
   Confirm an unauthorized runtime key cannot read the same tool's schema or call it.

If discovery is empty, check publication, runtime vMCP assignments, approved
upstream grants and index refresh. HTTP 401 means invalid/revoked credentials;
403/404 means unavailable or unauthorized target; 409 means refresh the schema or
retry after a configuration change; 429 means concurrency/search quota; 503 means
an unavailable index, dependency or authorization snapshot. Never automatically
retry a write just because a response was lost. Embedding outages fall back to
keyword search; inspect `search_mode` and index status rather than assuming semantic
search is active.

## Embedding diagnostics in pod logs

The diagnostic logging below is available from v1.2.2; v1.2.1 contains the TLS
flag but not these diagnostic records.

```sh
kubectl logs -n <namespace> <omni-pod> --since=15m -f
# If the process restarted:
kubectl logs -n <namespace> <omni-pod> --previous
```

No debug flag is required. Startup logs `omni embedding configuration` with
`enabled` and `tls_skip_verify`, making missing embedding configuration visible.
Failures appear at WARN as `omni dependency failed` with an `operation`:

- `index_refresh`: embedding batch failed; includes batch offset/size, pending
  document count, total documents, elapsed milliseconds and keyword fallback.
- `search_embedding`: query embedding failed and keyword fallback was selected.
- `catalog_refresh`: the data-plane catalog could not be read; HTTP status is
  included when available. Embedding TLS settings do not affect this connection.

Examples (text logger; JSON handlers represent `error` as an object):

```text
level=WARN msg="omni dependency failed" operation=index_refresh error.reason=tls_unknown_authority error.provider_origin=https://embeddings.internal error.tls_skip_verify=false batch_offset=0 batch_size=32 fallback=keyword
level=WARN msg="omni dependency failed" operation=index_refresh error.reason=http_status error.http_status=404 error.provider_code=model_not_found error.tls_skip_verify=true fallback=keyword
```

`error.reason` distinguishes unknown certificate authority, hostname mismatch,
invalid certificate, invalid TLS record, DNS failure, connection refusal/reset,
timeout, HTTP rejection, empty/truncated/malformed JSON, incorrect response types,
embedding count mismatch and invalid/inconsistent dimensions. HTTP failures include
status and, when provided, bounded machine-readable `provider_code`/`provider_type`.
Count/dimension failures include expected/actual counts or dimensions. Invalid JSON
includes its byte offset where available. For HTTP 401/403 check provider access;
404 often indicates a route/model mismatch; 429 indicates quota/concurrency; 5xx
indicates a provider/proxy failure. These statuses do not alone prove a root cause.

The first failure is logged immediately. Consecutive identical failures are
summarized every 30 seconds per operation with `suppressed_failures`; a changed
failure is logged immediately. A subsequent successful operation logs
`omni dependency recovered`. Cancellation by the search caller is not logged as a
provider outage. Recovery refers to that operation, not a global health guarantee.

API keys, request headers, query/tool text, raw provider bodies/messages and URL
paths/query strings are not logged. Machine-readable provider identifiers are
filtered and bounded. Provider-side logs may still be necessary when the service
returns no useful error code. Public search responses and index status retain
their existing safe error messages; internal diagnostics stay in process logs.

## Anonymous published servers

This option requires v1.2.2 or later.
Set `VMCP_OMNI_ALLOW_ANONYMOUS=true` on **both DP and Omni** and restart/recreate
both processes. In Helm set `omni.allowAnonymous: true`; Compose passes the same
environment variable to both services. The default is false and invalid boolean
values stop startup. No database migration is needed.

Anonymous clients omit the Authorization header. Only virtual servers with
anonymous runtime access **and** Omni publication enabled are available through
all four Omni tools. Protected or unpublished servers remain hidden. A valid
runtime credential sees published public tools plus its assigned protected tools;
credential IP restrictions still apply. Any supplied invalid/expired credential
is rejected rather than treated as anonymous. Runtime OAuth clients still need
a matching identity binding. Existing owner approvals, tool policies, limits,
auditing and fresh authorization checks continue to apply.

Discovery limits are per verified client IP for anonymous requests and per
credential for authenticated requests, independently on each Omni replica.
Configure trusted proxies accurately; users behind one NAT share an anonymous
search quota. Invocation uses existing per-IP anonymous concurrency limits and server limits.
Anonymous invocation is recorded with its client IP, target and Omni action.
The console preview still requires an explicit runtime credential; external MCP
clients can use the public endpoint without one when the feature is enabled.

## Optional reranking

For the separate model API contract, keyless/bearer configuration, TLS controls,
score calibration and closed/fallback behavior, see
[Omni reranking and no-match calibration](Omni-MCP-Reranking.md). This optional
feature is available from v1.2.2 and does not require a vector database or a database migration.
