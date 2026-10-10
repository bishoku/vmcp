# Deployment

## Minimum production checklist

1. **Secrets**: generate a strong `VMCP_ADMIN_TOKEN` and a fresh `VMCP_MASTER_KEY`; keep the key in a secret manager and
   back it up. Do not reuse the development defaults from the compose file.
2. **TLS**: terminate HTTPS in front of both the UI/control plane and the data plane (reverse proxy or ingress). vMCP
   itself speaks plain HTTP.
3. **Proxies**: set `VMCP_TRUSTED_PROXIES` on the data plane to your proxy CIDRs, otherwise IP allowlists and audit client IPs
   see the proxy.
4. **PostgreSQL**: use a managed or replicated instance with backups. It holds all state, including audit events.
5. **Network**: only the data plane needs to be reachable by agents; keep the control plane and UI on an internal or
   authenticated network. Data-plane nodes must reach the upstreams.
6. **Admin access**: prefer [SSO](SSO-and-RBAC.md) plus scoped admin tokens over sharing the bootstrap token.

## Services

Build images from `core/Dockerfile` (`--build-arg SERVICE=control-plane` or `data-plane`) and `web/Dockerfile`.
The control plane applies migrations on start; start it before (or together with) the data plane.

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

Migrations are forward-only and run automatically by the control plane. Take a database backup before upgrading, and
upgrade the control plane first. A Helm chart is not provided yet.

## Backups and portability

Besides database backups, `GET /api/v1/export` produces a declarative document (upstreams and virtual servers; no secrets)
that `POST /api/v1/apply` can load into another environment ([REST API and GitOps](REST-API.md)).
