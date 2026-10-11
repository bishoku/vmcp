# Development

## Layout

```
api/openapi.yaml   REST contract
core/              Go module (github.com/bishoku/vmcp/core)
  cmd/             control-plane, data-plane, fixture-mcp
  internal/        store, controlplane, virtual (data plane), upstream (MCP clients), mcp, policy, pii, resilience,
                   ratelimit, health, tlsconf, oidc, secrets, ...
  migrations/      goose SQL migrations (embedded)
web/               React + Vite + Tailwind UI
deploy/            Docker Compose
```

## Run tests

```sh
docker compose -f deploy/docker-compose.yml up -d postgres
export VMCP_TEST_DATABASE_URL='postgres://vmcp:vmcp@localhost:5432/vmcp_test?sslmode=disable'
cd core && go test -race -count=1 ./...
```

Database-backed suites skip without `VMCP_TEST_DATABASE_URL`. With `VMCP_TEST_REDIS_URL=redis://localhost:6379/0` the Redis
rate-limit test also runs. Integration tests in `core/internal/e2e` and `core/internal/controlplane` spin up real fixture
upstreams (including TLS/mTLS with generated certificates) and one or two data-plane nodes.

## UI and API tooling

```sh
cd web && npm ci && npm run dev        # proxies /api and /auth to :8081
npm run lint && npm run build
npx @redocly/cli lint api/openapi.yaml
```

## Conventions

- API first: UI features must exist in the API and OpenAPI document, and in export/apply where relevant.
- Add a migration for schema changes (`core/migrations`, next number); operational tables must not have revision triggers.
- Never log or return secrets. No option to disable TLS verification. Core features must not need Redis.
- See [CONTRIBUTING.md](https://github.com/bishoku/vmcp/blob/main/CONTRIBUTING.md).

## UI-first manual testing

[benchmark/](../benchmark/README.md) provides a separate Compose project with
PostgreSQL, Redis, imported Keycloak users, two data planes behind nginx and three
synthetic MCP servers. It starts with empty vMCP configuration. Run its optional
smoke check and fixture unit tests independently from the core verification gate.

## Public MCP connection URL in the UI

The API supplies each virtual server's MCP endpoint path. The UI combines it with
`VITE_PUBLIC_GATEWAY_URL`, an explicit public gateway base URL, instead of guessing
an origin or port. Without a valid base URL the UI displays only the path, disables
copying a full endpoint, and explains the missing configuration.

For local Vite development, add `VITE_PUBLIC_GATEWAY_URL=http://localhost:8080` to
`web/.env` (use your actual data-plane/gateway address). For the web Docker image:

```sh
docker build --build-arg VITE_PUBLIC_GATEWAY_URL=https://mcp.example.com -t vmcp/web:local web
```

This is public, non-secret **build-time** configuration; rebuild the image after
changing it. The manual-test Compose stack sets `http://localhost:8800`. The
`deploy/docker-compose.yml` web build uses `VMCP_PUBLIC_URL`, defaulting to
`http://localhost:8080`. Use the browser-reachable gateway address, not an internal
container hostname. HTTP is for local development; production should use HTTPS.
