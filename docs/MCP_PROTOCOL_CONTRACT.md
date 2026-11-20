# MCP protocol contract

vMCP uses **2025-06-18** by default. Starting with v2.3.0, set
`VMCP_MCP_20260728=1` on the control plane, every data-plane replica and
Omni MCP to enable **2026-07-28** for synchronous tool discovery and invocation.

## Versions and transports

| Hop | Supported scope |
|---|---|
| Client → Virtual or Omni | Legacy Streamable HTTP or opt-in modern requests |
| vMCP → native upstream | Independent revision selection on Streamable HTTP. The legacy HTTP+SSE transport continues to use the legacy handshake. |

## Tool calls across revisions

| Client | Legacy-only upstream | Modern or dual upstream with modern support enabled |
|---|---|---|
| Legacy Virtual | An open Virtual session can reuse its upstream session on the same replica. | vMCP prefers modern and sends the tool call without an upstream handshake. |
| Modern Virtual | Each tool invocation opens its own legacy upstream session. | Each tool invocation uses modern requests. |
| Omni, either client revision | Each `invoke_tool` opens its own legacy upstream session. | Each `invoke_tool` uses modern requests. |

On first use, a data-plane replica probes the upstream with `server/discover`.
It caches the revision preference for five minutes. A warm modern call can go
directly to `tools/call`. A fresh legacy session still requires `initialize`
and `notifications/initialized` before the tool runs.

**Multi-step tools.** Suppose `open_browser()` creates a browser on a legacy
upstream. To continue through Omni or modern Virtual, the upstream must keep
the browser alive after the first MCP session closes and make it accessible
from a new authorized session. For example, return a `browser_id` from
`open_browser()` and accept it in `navigate(browser_id, url)`. If the upstream
keeps only a “current browser” inside the first session, the second call cannot
resume it. Returning an ID alone cannot preserve a browser destroyed at session
close.

Legacy Virtual session records are shared through PostgreSQL. Upstream
connections and revision preferences are local to each replica. Moving a
legacy Virtual client to another replica can therefore establish a new
upstream session even when its downstream session ID stays the same.

## Legacy handshake

Unknown initialize versions receive the supported June version; clients decide
whether to continue. Unsupported upstream negotiated versions are rejected.
Handshake fields include protocolVersion, object capabilities and implementation
name/version. A known session supplies its version when the header is absent;
explicit unsupported or mismatched versions are rejected. Origin, Accept, media
and bounded-body checks apply to virtual gateway HTTP methods. See
[network boundaries](MCP_NETWORK_SECURITY.md).

## Legacy framing and lifecycle

- A body contains one JSON-RPC object. Batches and mixed request/response envelopes are rejected.
- Request IDs are strings or integer JSON tokens, never null/fractional; replies preserve their representation.
- Notifications omit IDs. Accepted notifications/client replies receive empty HTTP 202; unknown or cross-session reply IDs fail.
- Initialization creates a pending, persisted session; normal operations require `notifications/initialized`. Ping is allowed while pending; repeated initialized notifications are idempotent.
- Session ownership includes authentication mode, immutable credential identity and verified issuer/subject where relevant. Warm caches resolve authoritative PostgreSQL state.
- Mapped upstream IDs and progress tokens prevent response confusion across calls. GET/POST SSE events and pending replies have shared leases/fences and bounded encrypted storage.
- Last-Event-ID recovery is session/stream scoped. Disconnecting the gateway HTTP stream does not by itself cancel the operation. Cancellation cannot undo an external side effect, and lost workers do not imply safe automatic replay.

## Legacy feature families

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
