<div align="center">

# vMCP

**An open-source gateway for the Model Context Protocol.**

Register upstream MCP servers and REST APIs, then publish selected capabilities
through virtual MCP servers or a shared Omni discovery endpoint.

[![CI](https://github.com/bishoku/vmcp/actions/workflows/ci.yml/badge.svg)](https://github.com/bishoku/vmcp/actions/workflows/ci.yml)
[![License: Apache-2.0](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)

</div>

## Why vMCP?

Give each agent the capabilities it needs without distributing upstream credentials.
Use the web UI or REST API to compose virtual servers, rename capabilities and
manage access from one place.

- **Control access:** agent keys, identity-provider JWTs, tool policies and rate limits.
- **Protect data:** encrypted upstream credentials and configurable PII masking.
- **Investigate activity:** audit records and independent HTTPS, Kafka or RabbitMQ export.
- **Handle failures:** health checks, circuit breakers and bounded retries for eligible tools.
- **Scale out:** multiple data planes share PostgreSQL state; Redis is optional for global rate limits.

MCP upstreams must expose HTTP endpoints (Streamable HTTP or legacy HTTP+SSE).
REST upstreams expose configured HTTP operations as MCP tools.
vMCP does not launch local stdio servers. Client connections use Streamable HTTP,
with the **2025-06-18** protocol baseline and negotiated optional features.

## Choose your starting point

| Goal | Guide |
|---|---|
| Try the UI with bundled sample MCP servers | [Getting started](docs/Getting-Started.md) |
| Run a smaller local stack with your own upstream | [Local Docker installation](docs/Deployment.md#local-docker-installation) |
| Try vMCP on an existing development Kubernetes cluster | [Kubernetes quickstart](docs/HELM_QUICKSTART.md) |
| Install a published version | [Release installation](docs/Release-Installation.md) |
| Plan a production deployment | [Production deployment](docs/PRODUCTION_DEPLOYMENT.md) |

For the guided local demo, clone the repository and start the sandbox:

```sh
git clone https://github.com/bishoku/vmcp.git
cd vmcp
docker compose -f benchmark/compose.yaml up -d --build
```

Requires Docker with Compose, roughly 6–8 GiB available memory and internet access
for the initial build. Open **http://localhost:3300** and sign in with the local demo
admin token `demo-admin-token`. The sandbox uses public development credentials;
use it locally. Follow [Getting started](docs/Getting-Started.md) to register a
sample upstream, create a virtual server and connect with an agent key.

## How it works

```mermaid
flowchart LR
    subgraph CLIENTS["Clients"]
      direction TB
      A["Agent / MCP client"]
      ADMIN["Operator / API client"]
    end

    subgraph VMCP["vMCP services"]
      direction TB
      subgraph CORE["Required core"]
        CP["Control plane<br/>Configuration and permissions"]
        DP["Data plane<br/>Authorization, policies and execution"]
      end
      subgraph OPTIONAL["Optional services"]
        UI["Web UI"]
        OMNI["Omni MCP<br/>Tool discovery and invocation"]
        EX["Audit exporters<br/>Independent workers"]
      end
    end

    subgraph STATE["State and shared infrastructure"]
      direction TB
      PG[("PostgreSQL<br/>Required")]
      R[("Redis<br/>Optional global rate limits")]
    end

    subgraph EXTERNAL["External services · self-hosted or managed"]
      direction TB
      subgraph UPSTREAMS["Upstreams · choose per tool"]
        MCP["MCP servers<br/>Streamable HTTP / SSE"]
        REST["REST APIs<br/>HTTP operations"]
      end
      subgraph IDENTITY["Optional identity integrations"]
        IDP["Agent OAuth provider"]
        SSO["Console OIDC provider"]
        AUTH["Upstream OAuth provider"]
      end
      subgraph MODELS["Optional Omni model services"]
        EMB["Embedding model API<br/>Semantic retrieval"]
        RERANK["Reranker model API<br/>Candidate ranking"]
      end
      subgraph SINKS["Optional audit destinations"]
        HTTP["HTTPS endpoint"]
        KAFKA["Kafka"]
        RMQ["RabbitMQ"]
      end
    end

    A -->|Agent key / OAuth JWT| DP
    A -.->|Agent key / OAuth JWT| OMNI
    ADMIN -->|Management API| CP
    ADMIN -.-> UI
    UI -.-> CP
    OMNI -.->|Original caller identity| DP
    CP <-->|Configuration, sessions, audit| PG
    DP <-->|Configuration, delivery, audit| PG
    DP -.->|Shared quotas| R
    PG -.->|Persisted audit events| EX
    EX -.->|JSON POST| HTTP
    EX -.->|Produce| KAFKA
    EX -.->|Publish| RMQ

    DP -->|MCP| MCP
    DP -->|HTTP| REST
    A -.->|Obtain access token| IDP
    DP -.->|Discovery / JWKS| IDP
    CP -.->|Authorization code + PKCE| SSO
    DP -.->|Client credentials| AUTH
    CP -.->|Discovery / test credentials| AUTH
    OMNI -.->|Embed catalog and query| EMB
    OMNI -.->|Rank authorized candidates| RERANK

    classDef core fill:#dbeafe,stroke:#2563eb,color:#172554,stroke-width:2px
    classDef optional fill:#ede9fe,stroke:#7c3aed,color:#2e1065,stroke-width:1.5px,stroke-dasharray:5 4
    classDef state fill:#ccfbf1,stroke:#0f766e,color:#134e4a,stroke-width:2px
    classDef optionalState fill:#ccfbf1,stroke:#0f766e,color:#134e4a,stroke-dasharray:5 4
    classDef external fill:#f1f5f9,stroke:#64748b,color:#0f172a
    classDef integration fill:#fff7ed,stroke:#c2410c,color:#7c2d12,stroke-dasharray:5 4
    class A,ADMIN,MCP,REST external
    class CP,DP core
    class UI,OMNI,EX optional
    class PG state
    class R optionalState
    class IDP,SSO,AUTH,EMB,RERANK,HTTP,KAFKA,RMQ integration
    style VMCP fill:#eff6ff,stroke:#2563eb,stroke-width:2px,color:#172554
    style CORE fill:#ffffff,stroke:#93c5fd,color:#172554
    style OPTIONAL fill:#faf5ff,stroke:#a78bfa,stroke-dasharray:5 4,color:#2e1065
    style STATE fill:#f0fdfa,stroke:#5eead4,color:#134e4a
    style CLIENTS fill:#f8fafc,stroke:#cbd5e1,color:#0f172a
    style EXTERNAL fill:#f8fafc,stroke:#94a3b8,color:#0f172a
    style UPSTREAMS fill:#ffffff,stroke:#cbd5e1,color:#0f172a
    style IDENTITY fill:#fffaf5,stroke:#fdba74,stroke-dasharray:5 4,color:#7c2d12
    style MODELS fill:#fffaf5,stroke:#fdba74,stroke-dasharray:5 4,color:#7c2d12
    style SINKS fill:#fffaf5,stroke:#fdba74,stroke-dasharray:5 4,color:#7c2d12
```

Blue components form the required vMCP core; teal denotes shared state. Purple
components and orange integrations are optional. Dashed borders and arrows mark
optional components and connections; solid arrows show core paths and configured
upstream calls. External services can run in your own infrastructure or be managed
by another provider.

Omni works with keyword search alone. An optional
[embedding model API](docs/Omni-MCP-Embedding.md) adds semantic retrieval, while an
independently optional [reranker API](docs/Omni-MCP-Reranking.md) scores authorized
candidates before results are returned. Models run behind separately deployed APIs;
vMCP does not bundle model weights.

Agents connect directly to a virtual server at `/mcp/v1/virtual/{slug}` or use
[Omni](docs/Omni-MCP.md) to discover and call published tools through one MCP
endpoint. Both paths use the data plane's authorization, policies and upstream
execution. MCP servers and configured REST operations can supply those tools.

The control plane manages configuration and console permissions. PostgreSQL holds
configuration, shared session/delivery state and audit records; upstream connections
and some caches remain local to each node. Independent
[audit exporters](docs/AUDIT_EXPORTERS.md) read persisted events and deliver them
to HTTPS, Kafka or RabbitMQ destinations with at-least-once delivery.

Optional identity integrations serve three separate purposes:
[agent OAuth](docs/OAuth-for-Agents.md) validates access tokens at the gateway,
[console OIDC](docs/SSO-and-RBAC.md) provides browser SSO, and
[upstream OAuth](docs/Upstream-OAuth.md) obtains service tokens with client
credentials. These providers may share an identity platform but use separate
configuration and trust boundaries. Omni preserves the original caller identity
for data-plane checks; agent access tokens are not forwarded to upstream services.
See [Architecture](docs/Architecture.md) for state and deployment details.

## Documentation

Start at the [documentation home](docs/Home.md), also published as the
[GitHub Wiki](https://github.com/bishoku/vmcp/wiki).

- [Guided setup and client connection](docs/Connections-and-Setup.md)
- [Upstreams](docs/Upstreams.md) and [virtual servers](docs/Virtual-Servers.md)
- [Authentication](docs/Agent-Keys-and-Authentication.md), [SSO and roles](docs/SSO-and-RBAC.md), [security](docs/Security.md)
- [Audit log](docs/Audit-Log.md), [observability](docs/OBSERVABILITY.md), [troubleshooting](docs/FAQ.md)
- [Configuration reference](docs/Configuration.md), [REST API and GitOps](docs/REST-API.md), [protocol compatibility](docs/MCP_PROTOCOL_CONTRACT.md)

## Status and limits

vMCP is **Beta / development**; APIs may change. There is no declared production
SLA or compliance certification. Production use requires deployment-specific
load, failover, backup/restore and security validation described in the
[production guide](docs/PRODUCTION_DEPLOYMENT.md).

PII masking is configurable regex inspection, not a guarantee against data leaks
or prompt injection. Ambiguous side-effecting operations must be reconciled before
retrying. Tenant isolation uses dedicated plane/database deployments.
See [known limitations](docs/FAQ.md#known-limitations).

## Contributing

Read [CONTRIBUTING.md](CONTRIBUTING.md) for development, CI and wiki publishing.
[Maintainer documentation](maintainer/README.md) covers release publishing,
implementation history and the production readiness backlog.
Report vulnerabilities as described in [SECURITY.md](SECURITY.md).
Participation follows the [Code of Conduct](CODE_OF_CONDUCT.md).

## License

[Apache-2.0](LICENSE). Copyright 2026 Barış Oku.

Try upstream/tool step-up authentication and operation approval with `./deploy/helm/vmcp/quickstart.sh --security-demo`. The isolated local lab includes Keycloak OTP, an approval service and a browser MCP client with selectable OAuth-user and shared-application/caller-context modes. See [Invocation security](docs/Invocation-Security.md).
