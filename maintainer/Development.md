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
runtime `window.__VMCP_CONFIG__.publicGatewayURL` (or the development fallback
`VITE_PUBLIC_GATEWAY_URL`), an explicit public gateway base URL, instead of guessing
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

### Frontend interaction and authentication

The UI prefers SSO when configured. Token login keeps the credential only in
memory: reloading or closing the tab requires signing in again, and legacy
`vmcp.adminToken` local storage is removed on startup. Nothing persists form drafts
or write-only credentials to browser storage.

Dirty editors guard dismissal, entity switching, route navigation, browser Back,
sign-out and page unload. Failed writes retain their draft; a stale-version error
offers an explicit discard-and-reload action rather than overwriting newer data.
An expired session allows token reauthentication while retaining the open draft;
SSO redirects reload the page, so copy changes before taking that route. Mutations
are never automatically replayed after authentication or conflicts.

Viewers can inspect configuration and filter/refresh lists, but cannot mutate it.
Editors can change configuration (including configuration import); only admins
can manage admin API tokens. These controls supplement server authorization.
Independent page catalogs report partial failures and offer refresh; obsolete
reads are cancelled and ignored even if a source completes after cancellation.
Audit filters debounce reads and protect against out-of-order responses.

Run `npm --prefix web run <script>` with each of `test`, `lint`, `build`,
`validate:api` and `test:browser` for frontend verification.
Browser tests use mocked APIs; the separate `test:live` suite exercises a disposable
stack. See the verification instructions below and the [implementation history](FRONTEND_IMPLEMENTATION_PLAN.md).


### Guided setup and connections

See [Connections and setup](../docs/Connections-and-Setup.md) for the configuration flow,
focused virtual-server views, explicit SDK verification and browser CORS setup.
The MCP SDK is now a runtime dependency; its verification module loads only on
request. Playwright sets an explicit loopback gateway URL and mocks MCP traffic.
The demo proxy CORS regression runs separately with
`python3 benchmark/test_gateway_cors.py` using an isolated Docker container.

The production web image listens on port 8080 as an unprivileged user. Set
`VMCP_CONTROL_PLANE_URL` at container startup to configure its nginx proxy. Mount
a public `/usr/share/nginx/html/runtime-config.js` containing
`window.__VMCP_CONFIG__ = {"publicGatewayURL":"https://mcp.example.com"};` to reuse
one image across environments. The Helm chart manages this ConfigMap for you.


## Verification scope

Unit/component regressions cover relationship semantics, inactive credentials,
anonymous access, alias/policy display, missing objects, partial failures, large
edge sets, saved views, setup steps, activity windows and SDK message sequences.
Chromium regressions cover keyboard inspection, configuration deep links, dirty
navigation and deliberate discovery using a separate gateway credential.
`python3 benchmark/test_gateway_cors.py` runs an isolated Nginx CORS regression
with Docker and removes its temporary container. It does not modify the demo stack.
These checks do not replace live-stack, responsive or accessibility testing.




## Frontend verification

From the repository root:

```sh
npm --prefix web run test
npm --prefix web run lint
npm --prefix web run build
npm --prefix web run validate:api
npm --prefix web run test:browser
npm --prefix web run test:live
```

Install dependencies with `npm --prefix web ci` and install Chromium with
`cd web && npx playwright install chromium` before browser checks. Standard browser
tests mock API responses to exercise draft recovery, permissions, keyboard flows,
audit investigation and layouts at 375/768/1440px. They capture screenshots in
`web/test-results/` and check selected theme contrast pairs; they are not a complete
accessibility audit or pixel-diff suite.

`test:live` requires Python 3, Docker with Compose supporting `--wait` and JSON
configuration, and enough capacity to build/run the full demo. It resolves
`benchmark/compose.yaml` into a uniquely named project with new volumes, loopback
ports, network and build image tags. It uses development-only synthetic credentials.
The default isolated subnet is `172.30.73.0/24`; set `VMCP_UI_TEST_SUBNET` to another
unused private CIDR if needed. Do not run concurrent live tests on the same subnet.
The test seeds a real upstream, resource/prompt selection and key through the API,
then verifies login and MCP discovery from the built UI in Chromium. It exercises
both configured data planes behind the gateway, but does not assert every request
reaches a different replica or test SSO. Screenshots are in `web/test-results-live/`.
A finally block removes its containers, network, volumes and image tags, including
on failure; existing demo data and images are not used as test state. An externally
terminated Python process may require cleaning up the printed `vmcp-ui-*` project.
Rebuild your manual demo's web image to see the new UI; running it was not changed
by these tests.


## Reproducible verification

Use the Go version in `core/go.mod`, Node 22 or newer and Docker. From a clean
checkout run `npm ci --prefix web` and `cd web && npx playwright install chromium`.
Start disposable PostgreSQL 17 and Redis 7, set `VMCP_TEST_DATABASE_URL` (a role
allowed to create/drop temporary test databases) and `VMCP_TEST_REDIS_URL`, then
run `make verify`. Never point these variables at production infrastructure.
The release command sets `VMCP_REQUIRE_INTEGRATION=1` and
`VMCP_REQUIRE_SDK_INTEROP=1` and rejects skipped tests in the Go JSON results.
CI runs the same race, vet, frontend, browser and OpenAPI checks, plus dependency
and container scanning. `make images` builds planes and operational commands.

The pinned TypeScript MCP SDK is `@modelcontextprotocol/sdk` **1.32.1**. Its real
HTTP client negotiates **2025-06-18**, initializes, pings, lists/calls tools and
terminates a gateway session in `TestPinnedTypeScriptSDKInterop`. This does not
certify every SDK feature or claim support for newer protocol revisions. The
other integration tests cover bidirectional replay, resources/prompts and client
feature negotiation with gateway fixtures. Update the pin deliberately and
repeat compatibility tests. No March batching compatibility is advertised.
