# Installation options

| Your goal | Recommended path |
|---|---|
| Explore the UI with sample servers | [Getting started](Getting-Started.md) |
| Run core services with your own HTTP upstream | Local Docker installation below |
| Try Kubernetes with bundled dependencies | [Kubernetes quickstart](HELM_QUICKSTART.md) |
| Install a versioned packaged chart and images | [Release installation](Release-Installation.md) |
| Operate a production environment | [Production deployment](PRODUCTION_DEPLOYMENT.md) |

## Local Docker installation

Requires Git and Docker with Compose; the images build from source. From a clone:

```sh
git clone https://github.com/bishoku/vmcp.git
cd vmcp
docker compose -f deploy/docker-compose.yml up -d --build
docker compose -f deploy/docker-compose.yml ps
```

Open **http://localhost:3000** and sign in with `dev-admin-token`. The API is at
`http://localhost:8081/api/v1`; MCP endpoints use
`http://localhost:8080/mcp/v1/virtual/{slug}`. Wait for both planes' `/readyz`
endpoints to return success before configuring clients.

This stack contains PostgreSQL, the planes and the UI, with no sample upstreams.
Use [Guided setup](Connections-and-Setup.md) with your own HTTP MCP endpoint.
Internal/loopback upstreams require explicit outbound network grants on both
planes; follow [network configuration](MCP_NETWORK_SECURITY.md#development-and-deployment-compatibility).
Browser verification across UI/gateway origins also requires a CORS-capable proxy;
see [browser verification](Connections-and-Setup.md#deliberate-browser-verification).
The [guided demo](Getting-Started.md) already includes that proxy and sample servers.

The Compose file uses public development secrets. Set your own admin token and
master key before exposing the services; see [Security](Security.md).
Stop with `docker compose -f deploy/docker-compose.yml down`. Adding `-v` also
removes the database volume and permanently deletes this stack's stored data.

## Minimum production checklist

1. **Secrets**: generate a strong `VMCP_ADMIN_TOKEN` and a fresh `VMCP_MASTER_KEY`; keep the key in a secret manager and
   back it up. Do not reuse the development defaults from the compose file.
2. **TLS**: terminate HTTPS in front of both the UI/control plane and the data plane (reverse proxy or ingress). the API planes speak plain HTTP; the optional tenant router has its own TLS listener.
3. **Proxies**: set `VMCP_TRUSTED_PROXIES` on the data plane to your proxy CIDRs, otherwise IP allowlists and audit client IPs
   see the proxy.
4. **PostgreSQL**: use a managed or replicated instance with backups. It holds all state, including audit events.
5. **Network**: only the data plane needs to be reachable by agents; keep the control plane and UI on an internal or
   authenticated network. Data-plane nodes must reach the upstreams.
6. **Admin access**: prefer [SSO](SSO-and-RBAC.md) plus scoped admin tokens over sharing the bootstrap token.

## Services

Build images from `core/Dockerfile` (`--build-arg SERVICE=control-plane` or `data-plane`) and `web/Dockerfile`.
For coordinated startup, run the `migrate` command to completion before starting
the planes with `VMCP_SKIP_MIGRATIONS=true` on the control plane. Both planes check
the schema and dedicated tenant identity. The control plane can also migrate on
startup for development. See [production deployment](PRODUCTION_DEPLOYMENT.md).

## Scaling out

Run several data-plane nodes against the same database. They share sessions through PostgreSQL, so a session may
continue on any node. Put a load balancer in front; affinity on the `Mcp-Session-Id` header is nice to have (it avoids
re-opening upstream sessions) but not required. Details in [Resilience and high availability](Resilience-and-HA.md).

Optional Redis ([Rate limits](Rate-Limits.md)) is only needed for *global* limits; the compose file has a `redis` profile:

```sh
docker compose -f deploy/docker-compose.yml --profile redis up -d
# and set VMCP_REDIS_URL=redis://redis:6379/0 on the data plane
```

## Upgrades

Migrations use a dedicated connection and PostgreSQL advisory lock. Prefer the
deployment migration job before replacing planes; take a database/key backup first.
Rollback compatibility is migration-specific and may require a coordinated maintenance
window. A [Helm chart](../deploy/helm/vmcp) provides UI and plane replicas, safe install/upgrade migrations, optional dependencies,
probes, disruption budgets and network policies. HA/restore drills remain deployment
acceptance gates, not guarantees supplied by the chart.

## Backups and portability

Besides database backups, `GET /api/v1/export` produces a declarative document (upstreams and virtual servers; no secrets)
that `POST /api/v1/apply` can load into another environment ([REST API and GitOps](REST-API.md)).

## Legacy session schema upgrade

Drain old data-plane nodes before migration 17, replace every data-plane binary, and have clients initialize new sessions before restoring traffic. Legacy sessions lack a trustworthy subject binding and are rejected. Mixed versions and rollback need coordinated handling; see [Authorization and session consistency](AUTHORIZATION_CONSISTENCY.md#upgrade-and-rollback).

## Legacy audit schema upgrade

Apply schema 18 before starting the new data plane. Drain and replace all legacy writers before claiming strict coverage. Strict tool admission requires durable PostgreSQL storage; failures return 503. Archive audit rows and exporter checkpoints before rollback, which removes lifecycle metadata and started rows. See [Audit durability](AUDIT_DURABILITY.md#retention-export-and-rollout).
