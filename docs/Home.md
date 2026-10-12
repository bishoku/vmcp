# vMCP

vMCP is an open-source gateway for the [Model Context Protocol](https://modelcontextprotocol.io). You register
**upstream** MCP servers once and curate their tools, resources, templates and prompts into **virtual servers**: one endpoint per agent or team that
exposes only the selected capabilities, with authentication, PII masking, auditing, tool policies, rate limits and
failover in between.

## Start here

- [Getting started](Getting-Started.md): run it in five minutes
- [Architecture](Architecture.md): control plane, data plane, PostgreSQL
- [Configuration reference](Configuration.md): every environment variable
- [Deployment](Deployment.md): production notes, scaling out

## Concepts and features

- [Upstreams](Upstreams.md), [Upstream TLS](Upstream-TLS.md), and [Upstream OAuth](Upstream-OAuth.md)
- [Virtual servers](Virtual-Servers.md): selecting, renaming and re-describing tools
- [Tool policies](Tool-Policies.md): allow / deny / read-only
- [Agent keys and authentication](Agent-Keys-and-Authentication.md) and [OAuth for agents](OAuth-for-Agents.md)
- [SSO and roles](SSO-and-RBAC.md)
- [PII masking](PII-Masking.md)
- [Audit log](Audit-Log.md)
- [Resilience and high availability](Resilience-and-HA.md)
- [Rate limits](Rate-Limits.md)

## Reference

- [REST API and GitOps](REST-API.md)
- [Security](Security.md)
- [Development](Development.md)
- [FAQ and troubleshooting](FAQ.md)

> The source of truth for the API is [`api/openapi.yaml`](https://github.com/bishoku/vmcp/blob/main/api/openapi.yaml).

## Protocol and operations

- [MCP protocol contract](MCP_PROTOCOL_CONTRACT.md) and [bidirectional SSE delivery](MCP_DELIVERY.md)
- [Resources and prompts](MCP_CATALOGS.md) and [negotiated client features](MCP_CLIENT_FEATURES.md)
- [Audit durability](AUDIT_DURABILITY.md) and [SIEM/retention operations](AUDIT_OPERATIONS.md)
- [Observability](OBSERVABILITY.md), [security operations](SECURITY_OPERATIONS.md) and [production deployment](PRODUCTION_DEPLOYMENT.md)
- [Dedicated tenant operations](TENANT_OPERATIONS.md)
- [Manual testing stack](../benchmark/README.md)
- [Production roadmap](../README.md#roadmap-to-production-readiness)
