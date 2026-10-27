# Install Omni MCP without Helm

Omni MCP is an optional third process, alongside the control plane and data plane.
Use matching **v1.2.0** binaries/images for all three. The new image is
`barishoku/vmcp-omni-mcp:v1.2.0` (linux/amd64 and linux/arm64); pin the release
manifest's digest in production. Existing vMCP endpoints continue to work.
See [Omni behavior and security](Omni-MCP.md) before publishing tools.

## Upgrade an existing installation

1. Back up PostgreSQL and retain the existing master encryption key.
2. Apply migrations using the v1.2.0 `migrate` binary/image and the migration DB
   credential. Migration 34 adds `omni_enabled DEFAULT false`; migration 33 adds
   teams/projects if upgrading from before v1.1.0. Never run a second migration
   job concurrently. Existing control-plane automatic migration is also supported.
3. Upgrade control plane, data plane and web to v1.2.0. Set
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
| `VMCP_OMNI_SECRET` | CP, DP, Omni | Same random secret, at least 32 characters; required when enabled |
| `VMCP_OMNI_DATA_PLANE_URL` | Omni | Required internal DP base URL, e.g. `http://data-plane:8080`; no MCP suffix |
| `VMCP_OMNI_URL` | CP | Internal Omni base URL, e.g. `http://omni-mcp:8090`; enables management status/preview |
| `VMCP_OMNI_PUBLIC_URL` | CP, Omni | Public base URL, e.g. `https://mcp.example.com`; no MCP suffix |
| `VMCP_LISTEN` | Omni | `:8090`; bind `127.0.0.1:8090` for same-host proxy deployments |
| `VMCP_OMNI_REFRESH_INTERVAL` | Omni | `30s`, minimum `1s`; asynchronous catalog refresh |
| `VMCP_OMNI_SEARCH_LIMIT` | Omni | `120` searches per credential per minute **per replica**, positive integer |
| `VMCP_OMNI_EMBEDDING_URL` | Omni | Optional complete OpenAI-compatible embeddings URL, including `/v1/embeddings` |
| `VMCP_OMNI_EMBEDDING_MODEL` | Omni | Required when an embedding URL is set |
| `VMCP_OMNI_EMBEDDING_KEY` | Omni | Optional provider bearer credential; never a runtime agent key |
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
  barishoku/vmcp-omni-mcp:v1.2.0
curl --fail http://127.0.0.1:8090/readyz
```

The 512MiB soft / 1GiB container settings are a starting point for small catalogs;
monitor actual usage and embedding dimensions. They are independent of the
previously sized DP 4GiB soft / 6GiB container settings. The index is in memory and
rebuilt on restart; no volume is required. Restart rebuild can incur embedding
provider requests. Each replica has its own index and search quota.

### Local Docker Compose

The repository overlay builds the current source. Checkout the v1.2.0 tag to
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
