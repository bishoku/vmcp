# REST API and GitOps

The contract is [`api/openapi.yaml`](https://github.com/bishoku/vmcp/blob/main/api/openapi.yaml) (OpenAPI 3.1). Base path
`/api/v1`, authenticated with `Authorization: Bearer <admin token>` (or an SSO cookie from the UI).

## Conventions

- Resources are addressed by **slug**: `/upstreams/{slug}`, `/virtual-servers/{slug}`, `/agent-keys/{slug}`.
- `PUT` is an idempotent create-or-replace; `POST` on the collection creates (409 if it exists); `PATCH` is a JSON merge patch.
- Optimistic concurrency: reads return an `ETag`; send `If-Match` on writes to avoid lost updates (`412` on mismatch).
- Errors are `application/problem+json` with stable `code`s and per-field `errors[]` (JSON-pointer `field`s).
- Lists use cursor paging: `?limit=` and `?cursor=`, with `nextCursor` in the response.
- Secrets are write-only and appear once at creation: upstream headers and client keys never, agent/admin keys only in the
  creation or rotation response.

## Endpoint overview

| Area | Endpoints |
|---|---|
| Info | `GET /info`, `GET /me` |
| Upstreams | CRUD on `/upstreams` (with optional `auth` block for OAuth client credentials), `GET /upstreams/{slug}/tools`, `POST /upstreams/{slug}/refresh-tools`, `POST /connection-tests` |
| Virtual servers | CRUD on `/virtual-servers` |
| Identity bindings | CRUD on `/identity-bindings`, dry-run `POST /identity-bindings/evaluate` |
| Agent keys | CRUD on `/agent-keys`, `POST .../rotate`, `POST .../revoke` |
| Admin tokens | CRUD on `/admin-tokens`, `POST .../rotate`, `POST .../revoke` |
| Audit | `GET /audit-events` |
| Declarative | `GET /export`, `POST /apply` |

## Export and apply (GitOps)

`GET /api/v1/export` returns upstreams, virtual servers, and identity bindings as one document, without any secret values (headers, client keys, and OAuth client secrets are omitted). `POST /api/v1/apply`
loads such a document atomically in one transaction:

| Query | Effect |
|---|---|
| `dryRun=true` | Validate and report what would change, then roll back |
| `prune=true` | Delete upstreams, virtual servers, and identity bindings missing from the document (if `bindings` is provided) |
| `refreshTools=false` | Skip fetching tool schemas of new or changed upstreams (default `true`) |

The response lists each resource with `created`, `updated` or `unchanged`. A typical flow keeps the exported document in
Git and applies it from CI; secrets (upstream headers, client keys) are set separately through the API.

## Calling the data plane

Not part of `/api/v1`: MCP clients speak Streamable HTTP to
`/mcp/v1/virtual/{slug}` with `Authorization: Bearer <agent-key>` or an IdP-issued JWT.
See [Agent keys](Agent-Keys-and-Authentication.md) and [OAuth for agents](OAuth-for-Agents.md). RFC 9728 discovery is served at `/.well-known/oauth-protected-resource/mcp/v1/virtual/{slug}`.
