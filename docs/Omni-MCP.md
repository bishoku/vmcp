# Omni MCP

Omni is an optional, separately deployed entry point at `/mcp/v1/omni`. Existing
`/mcp/v1/virtual/{slug}` endpoints keep their current behavior. Omni exposes four
tools: `discover_server`, `discover_tools`, `get_tool_schema`, and `invoke_tool`.

## Publication and access

Migration 34 adds `omniEnabled`, false for every existing virtual server. Enable
**Publish through Omni MCP** in a virtual server's editor. The server description
and its published tool descriptions are search inputs. Existing description
overrides can improve search without maintaining another catalog.

Publication does not grant access. Agent keys and OAuth identity bindings retain
their virtual-server assignments, IP restrictions and expiration/revocation rules.
Omni always requires a runtime credential, including for vMCPs allowing anonymous
direct access. Projects remain organizational boundaries, not an additional set
of runtime grants. Upstream ownership approvals, policy filtering and PII-safe
published tool schemas remain authoritative.

References identify `(virtual server, exposed tool name)` within the configured
data-plane's tenant. They are not capabilities or secrets. Omni cannot select an
arbitrary data-plane or upstream URL. Deploy one Omni instance/group per tenant's
gateway boundary; cross-tenant federation is outside this version.

## Deployment

For Docker, Compose, native binaries, environment variables, reverse proxy and
upgrade steps, see [Install without Helm](Omni-MCP-Installation.md).

Build `core/cmd/omni-mcp`, or build `core/Dockerfile` with `SERVICE=omni-mcp`.
Helm supports an optional `omni` workload (disabled by default). It receives no
database credentials or master encryption key. The chart routes only the MCP
endpoint and OAuth resource metadata through public ingress, never `/internal/*`.

Minimum settings:

| Process | Setting | Purpose |
|---|---|---|
| All three | `VMCP_OMNI_SECRET` | Random shared internal credential, at least 32 characters |
| Omni | `VMCP_OMNI_DATA_PLANE_URL` | Fixed internal data-plane URL |
| Control-plane | `VMCP_OMNI_URL` | Fixed internal Omni URL |
| Omni / control-plane | `VMCP_OMNI_PUBLIC_URL` | Public base URL, without `/mcp/v1/omni` |
| Omni | `VMCP_AGENT_OIDC_ISSUER` | Same issuer as the gateway when OAuth is enabled |
| Omni | `VMCP_OMNI_EMBEDDING_URL` | Explicit embeddings HTTP endpoint, including its path |
| Omni | `VMCP_OMNI_EMBEDDING_MODEL` | Explicit model name |
| Omni | `VMCP_OMNI_EMBEDDING_KEY` | Optional provider credential |
| Omni | `VMCP_OMNI_REFRESH_INTERVAL` | Catalog refresh interval, default `30s` |
| Omni | `VMCP_OMNI_SEARCH_LIMIT` | Searches per credential per minute per replica, default `120` |
| Omni | `VMCP_LISTEN` | Listen address, default `:8090` (Helm uses `:8080`) |

Use TLS or a trusted, isolated cluster network for the internal links. Both the
component secret and the original runtime identity are required for caller
operations. The component secret alone can read the published indexing catalog
but cannot invoke tools. It must never be distributed to clients. Internal routes
must not be exposed by an external reverse proxy. Public ingress must strip or
ignore caller-supplied forwarding headers; configure `VMCP_TRUSTED_PROXIES` only
for actual trusted proxy peers.

Omni and data-plane are parts of **one logical OAuth protected gateway resource**.
The data-plane validates the original access token with its configured issuer and
audience on every operation. Configure the IdP and public resource URLs accordingly;
this implementation is not delegation between independent OAuth resources and
does not forward client tokens to an upstream service. Separate-resource token
exchange is not implemented. Upstream credentials continue to be supplied by the
data-plane's existing configuration.

For local Compose, layer `deploy/docker-compose.omni.yml` over the base compose
file and provide a random `VMCP_OMNI_SECRET`. The optional overlay binds Omni to
localhost. Do not use localhost HTTP as a production credential transport.

## Index and search

The control-plane remains the source of configuration. Rather than introducing a
second ownership/policy compiler there, the Omni worker reads the data-plane's
already-published catalog through an authenticated private interface. This is a
deliberate implementation simplification: approval filtering and description/PII
handling have one source of truth.

The index lives in Omni memory, with bounded document count (100,000), provider
batch size (32) and dimensions (4,096). It contains metadata and optional vectors,
not full schemas or compiled upstream clients. The data-plane caches only its published
metadata and hashes by snapshot revision; it never caches caller authorization.
Changed documents are re-embedded;
unchanged vectors are reused. Restart rebuilds the index. Each replica builds its
own index; large replica counts may justify a persistent shared index later.

Configure an OpenAI-compatible embeddings API, including a self-hosted model, to
use hybrid keyword + cosine search. There is **no implicit external provider**.
Without configuration the service explicitly reports `keyword`; provider failures
report degraded/fallback mode. Metadata and search queries are sent to the selected
embedding provider, never tool results or gateway credentials. Choose a provider
approved for that metadata. Search relevance requires evaluation on institution-
specific intent examples; no universal similarity threshold is assumed.

The index does not change runtime `config_revision`. Authorization is obtained
from a freshness-checked snapshot before scoring and checked again before returning
search results. Only matching current metadata hashes can be returned. Revoked,
unpublished or changed entries are hidden even when the index is old. Newly
published/changed entries may not appear until refresh. Schema and invocation
perform fresh target authorization independently. Authorization outages fail closed.

## Protocol and invocation

Example tool arguments:

```json
{"name":"discover_tools","arguments":{"query":"find a customer's open orders","limit":5}}
```

Use a returned `tool_ref` with `get_tool_schema`. Pass its `schema_version` with
`invoke_tool` to reject an outdated schema before execution. Successful invocation
returns the original MCP tool result, including `isError` and structured content;
it is not nested inside another tool result. The reference alone grants no access.

This first version implements stateless Streamable HTTP JSON request/response and
synchronous tools. It does not advertise sampling, elicitation, resources, prompts,
server-initiated streams, resumable delivery or async task capabilities. Each
invocation gets an isolated transient upstream session and uses the existing
call/policy/audit/limit path. This avoids sharing client capabilities or credentials
between agents but adds session setup cost for native MCP upstreams. REST upstreams
use the existing compiled REST path. Native MCP throughput must be measured before
assuming parity with long-lived direct MCP sessions. Writes are never automatically
retried by Omni.

The generic `invoke_tool` is conservatively marked potentially destructive and
non-idempotent. Agents must inspect the discovered schema/annotations and obtain
any necessary user approval. Search descriptions are untrusted metadata, not
instructions. Existing runtime approval hooks continue to apply.

## UI and operations

The **Omni MCP** screen shows endpoint and connection configuration. Platform
admins additionally see global index status and can request a refresh. Index refresh
is asynchronous and incremental; it does not mutate runtime configuration.

**Try discovery** requires an explicitly entered runtime credential. The browser
keeps it only in component memory and sends it in a dedicated request header, not
in the management audit payload or browser storage. Management session permissions
are never reused as runtime access. Preview can search and read schemas, not invoke.
Changing/clearing the credential clears results. Audit events for invocation use
`action=omni/tools/call` and retain the original runtime actor, target and trace ID.

Omni exposes `/healthz`, `/readyz`, and `/metrics`. HTTP counters/latency and index
readiness/document/revision gauges can be scraped on the private network. Discovery
has a separate per-replica credential rate limit and a 64-request concurrency bound;
existing invocation limits remain in the data-plane. It is not a distributed search
quota. Full authorized manifests are currently fetched for search, so discovery
load is additional data-plane work and is covered by the 1,000-tool acceptance profile.

## Local verification (2026-10-27)

- Migration/create/update/patch behavior verified against an isolated PostgreSQL 17.
- Native MCP search → schema → invocation verified with the pinned TypeScript SDK.
- OAuth audience, identity binding, client IP restrictions, revoked keys, removed
  tools, publication disablement and stale authorization snapshots covered by tests.
- A revocation between ranking and result delivery is tested explicitly.
- All Go packages passed `go test -race ./...` with PostgreSQL and WireMock available.
  Release CI additionally passed the Redis/Kafka/RabbitMQ integration suites.
- Web: 250 unit tests, 158 browser tests, build/lint and 73 OpenAPI operations passed.
- Helm rendering/regression and release artifact assembly tests passed.

An earlier development microbenchmark (before the final lexical-token correction;
Intel Xeon Silver 4216, amd64) measured the
ranking loop with an already-built index and authorization manifest, excluding
network, embedding, authorization fetch and tool execution:

| Documents | Initial implementation | Bounded top-K implementation | Allocated bytes/query after |
|---|---:|---:|---:|
| 6,000 | 6.19 ms/query | 0.48 ms/query | 2,816 |
| 60,000 | 81.72 ms/query | 6.97 ms/query | 2,816 |

These single local runs are diagnostic, not production latency/TPS acceptance.
Reproduce with `go test ./internal/omni -run '^$' -bench BenchmarkAuthorizedSearch -benchmem`
from `core/`.

The v1.2.0 end-to-end acceptance used 1,000 tools and 100 TPS in three ten-minute
phases, plus live bilingual relevance and loaded governance revocation checks.
See [results, provider fallback and resource costs](../benchmark/results/omni-1000-100-20261027.md).
The synthetic development corpus passed with the tested multilingual MPNet model;
institution-specific held-out relevance still requires separate validation.
