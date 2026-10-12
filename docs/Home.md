# vMCP documentation

vMCP lets you register HTTP MCP servers and expose selected tools, resources and
prompts through virtual servers. Agents connect to one endpoint with their own
credentials; upstream credentials remain in the gateway.

New to vMCP? Follow [Getting started](Getting-Started.md) for a local demo from
installation to authenticated discovery. Already have a running instance? Use
[Guided setup](Connections-and-Setup.md) to register upstreams and connect clients.
Choose [Installation options](Deployment.md) for Docker, Kubernetes or published releases.

vMCP is Beta / development. The supported MCP revision is 2025-06-18. Review
[known limitations](FAQ.md#known-limitations) and the [production guide](PRODUCTION_DEPLOYMENT.md)
before production use.

## Start here

- [Getting started](Getting-Started.md)
- [Guided setup and client connections](Connections-and-Setup.md)
- [Installation options](Deployment.md)
- [Kubernetes quickstart](HELM_QUICKSTART.md)
- [Install a published release](Release-Installation.md)

## Use vMCP

- [Upstreams](Upstreams.md)
- [Virtual servers](Virtual-Servers.md)
- [Resources and prompts](MCP_CATALOGS.md)
- [Agent keys and authentication](Agent-Keys-and-Authentication.md)
- [OAuth for agents](OAuth-for-Agents.md)
- [SSO and roles](SSO-and-RBAC.md)
- [Tool policies](Tool-Policies.md)
- [PII masking](PII-Masking.md)
- [Rate limits](Rate-Limits.md)
- [Audit log](Audit-Log.md)
- [FAQ and troubleshooting](FAQ.md)

## Deploy and operate

- [Production deployment](PRODUCTION_DEPLOYMENT.md)
- [Security](Security.md)
- [Network security](MCP_NETWORK_SECURITY.md)
- [Upstream TLS](Upstream-TLS.md)
- [Upstream OAuth](Upstream-OAuth.md)
- [Resilience and high availability](Resilience-and-HA.md)
- [Observability](OBSERVABILITY.md)
- [Audit operations](AUDIT_OPERATIONS.md)
- [Audit exporters](AUDIT_EXPORTERS.md)
- [Security operations](SECURITY_OPERATIONS.md)
- [Dedicated tenant operations](TENANT_OPERATIONS.md)

## Reference

- [Architecture](Architecture.md)
- [Configuration](Configuration.md)
- [REST API and GitOps](REST-API.md)
- [MCP protocol compatibility](MCP_PROTOCOL_CONTRACT.md)
- [SSE delivery and replay](MCP_DELIVERY.md)
- [Negotiated client features](MCP_CLIENT_FEATURES.md)
- [Authorization consistency](AUTHORIZATION_CONSISTENCY.md)
- [Audit durability](AUDIT_DURABILITY.md)

## Contribute

For source builds, tests, CI and publishing, see the repository's
[Contributing guide](../CONTRIBUTING.md) and [maintainer documentation](../maintainer/README.md).
The API contract is [OpenAPI](../api/openapi.yaml).
