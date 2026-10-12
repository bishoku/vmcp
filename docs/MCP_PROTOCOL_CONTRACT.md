# MCP protocol contract

The advertised protocol revision is **2025-06-18**. Support is capability-negotiated
and bounded as described below; this is not independent conformance certification.

## Versions and transports

| Hop | Supported scope |
|---|---|
| Client → vMCP | Streamable HTTP: POST JSON/SSE, GET SSE, DELETE sessions; June messages |
| vMCP → upstream | Streamable HTTP JSON/SSE; legacy HTTP+SSE carrying June messages |
| Other revisions | March, strict November 2024 and later revisions are not advertised |

Unknown initialize versions receive the supported June version; clients decide
whether to continue. Unsupported upstream negotiated versions are rejected.
Handshake fields include protocolVersion, object capabilities and implementation
name/version. A known session supplies its version when the header is absent;
explicit unsupported or mismatched versions are rejected. Origin, Accept, media
and bounded-body checks apply to virtual gateway HTTP methods. See
[network boundaries](MCP_NETWORK_SECURITY.md).

## Framing and lifecycle

- A body contains one JSON-RPC object. Batches and mixed request/response envelopes are rejected.
- Request IDs are strings or integer JSON tokens, never null/fractional; replies preserve their representation.
- Notifications omit IDs. Accepted notifications/client replies receive empty HTTP 202; unknown or cross-session reply IDs fail.
- Initialization creates a pending, persisted session; normal operations require `notifications/initialized`. Ping is allowed while pending; repeated initialized notifications are idempotent.
- Session ownership includes authentication mode, immutable credential identity and verified issuer/subject where relevant. Warm caches resolve authoritative PostgreSQL state.
- Mapped upstream IDs and progress tokens prevent response confusion across calls. GET/POST SSE events and pending replies have shared leases/fences and bounded encrypted storage.
- Last-Event-ID recovery is session/stream scoped. Disconnecting the gateway HTTP stream does not by itself cancel the operation. Cancellation cannot undo an external side effect, and lost workers do not imply safe automatic replay.

## Implemented feature families

| Family | Scope / limits |
|---|---|
| Tools | Curated list/call, aliases/descriptions, title/outputSchema/structuredContent/_meta, policy and quotas |
| Resources | Selected list/templates/read/subscribe/unsubscribe and update/list notifications; bounded physical watchers |
| Prompts/completion | Selected prompt list/get and upstream-supported prompt/template completion |
| Sampling | Explicit permission and client capability; host owns approval; no gateway model credentials or cross-server context aggregation |
| Roots | Negotiated host-provided file URI context; no gateway filesystem access; idle physical sessions receive changes on next use |
| Elicitation | Negotiated primitive forms, decline/cancel/expiry; no later URL mode |
| Logging | Shared level and sanitized static notification data, not raw upstream payload logs |

Capabilities reflect selected catalogs and implemented/negotiated behavior. Unknown
extensions are retained as metadata, not automatically executable features. Historical
client request-ID uniqueness remains the client's responsibility. See
[catalogs](MCP_CATALOGS.md), [delivery/replay limits](MCP_DELIVERY.md) and
[client features/consent](MCP_CLIENT_FEATURES.md).

## Verification and upgrades

`make verify` requires real PostgreSQL/Redis integration and pinned SDK interoperability,
Go vet/race tests, frontend checks, semantic API examples and a browser workflow.
CI separately runs real Docker sandbox checks. These are regression gates, not a
claim that every MCP client, optional feature combination or deployment failure has
been certified. The [manual stack](../benchmark/README.md) exercises curated catalogs,
progress, subscriptions and cross-node sessions with synthetic upstreams.

Use the coordinated migration command/job before replacing planes. Session lifecycle,
owner and delivery migrations require the documented cutover; legacy sessions may
need a fresh handshake. Review [authorization upgrades](AUTHORIZATION_CONSISTENCY.md),
[audit rollout](AUDIT_DURABILITY.md) and [production deployment](PRODUCTION_DEPLOYMENT.md)
before upgrades or rollback; do not assume mixed-version session compatibility.

Specification references: [June lifecycle](https://modelcontextprotocol.io/specification/2025-06-18/basic/lifecycle),
[June transports](https://modelcontextprotocol.io/specification/2025-06-18/basic/transports).
