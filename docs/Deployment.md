# Deployment

For a local UI-first walkthrough with Keycloak, Redis, two data planes and three
MCP servers, use [the manual testing stack](../benchmark/README.md). The stack is
development-only and does not constitute an HA PostgreSQL deployment.

## Minimum production checklist

1. **Secrets**: generate a strong `VMCP_ADMIN_TOKEN` and a fresh `VMCP_MASTER_KEY`; keep the key in a secret manager and
   back it up. Do not reuse the development defaults from the compose file.
2. **TLS**: terminate HTTPS in front of both the UI/control plane and the data plane (reverse proxy or ingress). the API planes speak plain HTTP; the separate stdio worker and tenant router have their own TLS listeners.
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
window. A [Helm chart](../deploy/helm/vmcp) provides plane replicas, migration hook,
probes, disruption budgets and network policies. HA/restore drills remain deployment
acceptance gates, not guarantees supplied by the chart.

## Backups and portability

Besides database backups, `GET /api/v1/export` produces a declarative document (upstreams and virtual servers; no secrets)
that `POST /api/v1/apply` can load into another environment ([REST API and GitOps](REST-API.md)).

## P03 session schema upgrade

Drain old data-plane nodes before migration 17, replace every data-plane binary, and have clients initialize new sessions before restoring traffic. Legacy sessions lack a trustworthy subject binding and are rejected. Mixed versions and rollback need coordinated handling; see [Authorization and session consistency](AUTHORIZATION_CONSISTENCY.md#upgrade-and-rollback).

## P04 audit schema upgrade

Apply schema 18 before starting the new data plane. Drain and replace all legacy writers before claiming strict coverage. Strict tool admission requires durable PostgreSQL storage; failures return 503. Archive audit rows and exporter checkpoints before rollback, which removes lifecycle metadata and started rows. See [Audit durability](AUDIT_DURABILITY.md#retention-export-and-rollout).
