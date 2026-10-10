# vMCP

vMCP is an open-source gateway for the [Model Context Protocol](https://modelcontextprotocol.io). You register
**upstream** MCP servers once and compose their tools into **virtual servers**: one endpoint per agent or team that
exposes only the tools it should see, with authentication, PII masking, auditing, tool policies, rate limits and
failover in between.

## Start here

- [Getting started](Getting-Started.md): run it in five minutes
- [Architecture](Architecture.md): control plane, data plane, PostgreSQL
- [Configuration reference](Configuration.md): every environment variable
- [Deployment](Deployment.md): production notes, scaling out

## Concepts and features

- [Upstreams](Upstreams.md) and [Upstream TLS](Upstream-TLS.md)
- [Virtual servers](Virtual-Servers.md): selecting, renaming and re-describing tools
- [Tool policies](Tool-Policies.md): allow / deny / read-only
- [Agent keys and authentication](Agent-Keys-and-Authentication.md)
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
