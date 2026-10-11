# Getting started

For a UI-first environment with Keycloak, Redis, two data planes and three MCP
servers exposing tools/resources/prompts, use [the manual testing stack](../benchmark/README.md).
The smaller walkthrough below demonstrates a single tools upstream.

## Run the stack

Requires Docker.

```sh
git clone https://github.com/bishoku/vmcp.git && cd vmcp
docker compose -f deploy/docker-compose.yml up -d --build
```

| Service | URL |
|---|---|
| Web UI | http://localhost:3000, sign in with the admin token `dev-admin-token` |
| Control plane (REST API) | http://localhost:8081/api/v1 |
| Data plane (MCP endpoints) | http://localhost:8080/mcp/v1/virtual/{slug} |

The compose file uses **development-only** secrets. Before exposing the stack, set `VMCP_ADMIN_TOKEN` and
`VMCP_MASTER_KEY` (see [Security](Security.md)).

Internal and loopback MCP servers now require an explicit `VMCP_EGRESS_ALLOW_CIDRS` grant on both services. Before using the host fixture below, authorize the actual IP/subnet that `host.docker.internal` resolves to inside your containers and recreate both services. See [network policy and development setup](MCP_NETWORK_SECURITY.md) for instructions. Public upstreams remain available by default; an exact host allowlist can restrict them further.

## Try it with the fixture server

The repository ships a tiny MCP server with four tools (`echo`, `add`, `whoami`, `fail`):

```sh
cd core && go run ./cmd/fixture-mcp -token up-secret     # http://localhost:9100/mcp
```

Register it, discover its tools and publish one of them:

```sh
A="Authorization: Bearer dev-admin-token"; B=http://localhost:8081/api/v1

curl -X PUT -H "$A" -H 'Content-Type: application/json' $B/upstreams/fx -d '{
  "name":"Fixture","url":"http://host.docker.internal:9100/mcp","transport":"streamable_http",
  "headers":{"Authorization":"Bearer up-secret"}}'

curl -X POST -H "$A" $B/upstreams/fx/refresh-tools

curl -X PUT -H "$A" -H 'Content-Type: application/json' $B/virtual-servers/demo -d '{
  "name":"Demo","authRequired":false,
  "tools":[{"upstream":"fx","tool":"add","alias":"sum"}]}'
```

(`host.docker.internal` lets the containers reach a server on your host. Adjust for your setup.)

Connect any MCP client to `http://localhost:8080/mcp/v1/virtual/demo`. You will see one tool, `sum`.

## Require authentication

Virtual servers require an agent key by default. Create one and send it as a bearer token:

```sh
curl -X POST -H "$A" -H 'Content-Type: application/json' $B/agent-keys -d '{
  "slug":"my-agent","virtualServers":["demo"]}'
# {"key":"vmcp_...", ...}   shown once
```

Use `Authorization: Bearer vmcp_...` on every data-plane request. Details: [Agent keys and authentication](Agent-Keys-and-Authentication.md).

## Using the UI

The UI covers everything the API does: **Upstreams** (add, test connection, refresh tools, health), **Virtual servers**
(drag to order tools, alias, description, policy, PII, rate limit), **Agent keys**, **Audit** and **Settings** (admin
tokens). With SSO configured the sign-in page also offers your identity provider ([SSO and roles](SSO-and-RBAC.md)).

## Next steps

[Virtual servers](Virtual-Servers.md) · [PII masking](PII-Masking.md) · [Resilience](Resilience-and-HA.md) ·
[Deployment](Deployment.md)
