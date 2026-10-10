# MCP protocol contract — P01

Status: implemented baseline, 10 October 2026. This decision narrows the advertised protocol contract to **2025-06-18**. It does not claim complete MCP conformance: Origin validation and other ingress/egress boundary work remain P02, and bidirectional client features remain P08–P10.

## Version and transport decisions

| Hop / transport | Contract |
|---|---|
| Client → vMCP, Streamable HTTP | June 2025; JSON responses; GET returns 405; DELETE terminates sessions. |
| vMCP → upstream, Streamable HTTP | June 2025; accepts JSON or SSE response streams. |
| vMCP → upstream, legacy HTTP+SSE | Legacy transport carrying the June 2025 message contract. A strict `2024-11-05` server is unsupported. |
| March 2025, either hop | No longer advertised. Its batching contract was not implemented. |
| Later protocol revisions / stdio | Unsupported until separately implemented and tested. |

An unknown initialize version receives the supported June version, allowing the client to decide whether to continue. Upstream initialize responses with unsupported versions are rejected before sending `notifications/initialized`. Required handshake fields are protocolVersion, an object of capabilities, and implementation information containing string name/version. Unknown extension capabilities are retained.

A known session supplies its negotiated version when the HTTP version header is absent. An explicit different version is rejected with HTTP 400 for session POST and DELETE. GET remains 405; broader ingress/header/Origin checks belong to P02. Without a known session, non-initialize POST returns 400 for a missing session ID or 404 for an unknown ID; it does not silently assume support for March.

Sources: [June lifecycle](https://modelcontextprotocol.io/specification/2025-06-18/basic/lifecycle), [June transport](https://modelcontextprotocol.io/specification/2025-06-18/basic/transports), [June base protocol](https://modelcontextprotocol.io/specification/2025-06-18/basic), [March transport](https://modelcontextprotocol.io/specification/2025-03-26/basic/transports).

## Envelope and lifecycle rules

- Decode exactly one JSON object. Malformed JSON returns ParseError; syntactically valid but invalid envelopes, including batches, return InvalidRequest.
- Request IDs are strings or integer JSON tokens, never null or fractional numbers. Preserve their wire representation when replying. Notifications omit ID.
- Requests/notifications require a nonempty method, optional object params, and no result/error fields. Responses require ID, exactly one object result or valid error object, and no method/params. Errors require integer code and string message.
- Match upstream responses to the originating request ID; reject unrelated JSON responses. SSE response streams also reject unrelated responses.
- Initialize creates a pending session. Reinitializing with an existing session ID is rejected. Ping is allowed while pending; normal operations require the initialized notification. Duplicate initialized notifications are idempotent and acknowledged with empty HTTP 202.
- Persist client capabilities and initialized state in PostgreSQL. Pending local caches recheck lifecycle state so initialization can complete on another node.
- Advertise only tools, with listChanged false. Constants for progress/cancellation do not advertise or implement those capabilities. Unsupported requests return MethodNotFound; otherwise accepted notifications receive empty 202. Unsolicited client responses return HTTP 400 because there are no pending server requests.

The gateway does not enforce uniqueness of all historical client request IDs; clients must obey that protocol requirement. Arbitrary extension capabilities are retained as data, not treated as implemented or forwarded features.

## Upgrade and rollback

Migration `00016_session_lifecycle.sql` adds initialized state and client capabilities. Apply migrations through the control plane before deploying new data-plane binaries. New columns are additive; older binaries ignore them. Avoid mixed old/new data-plane serving during cutover because old nodes do not enforce or persist the new lifecycle contract.

Existing sessions default to pending, and March sessions are rejected by upgraded nodes. Clients should reconnect and perform a full handshake. This compatibility change is deliberate; do not rely on existing sessions surviving deployment. Roll back binaries before dropping the columns. Client capabilities persist only for session lifetime and follow session retention, rather than becoming an audit payload.

## Regression matrix and verification

The existing CI `go test -race ./...` job executes these tests with PostgreSQL and Redis:

| Contract | Regression coverage |
|---|---|
| Message kinds, absent IDs, valid ID echo | `core/internal/mcp/jsonrpc_test.go`: TestDecodeKinds, TestIDRoundTripVerbatim, TestNotificationOmitsID |
| Parse errors vs invalid envelopes; null/fractional IDs; mixed envelopes; result/error validation | TestDecodeRejects, TestProtocolEnvelopeMatrix |
| Required initialization fields, extension preservation, June-only negotiation | TestInitializeContractMatrix, TestNegotiateVersion |
| JSON/SSE matching and malformed upstream responses | `core/internal/upstream/client_test.go`: TestClientRejectsUnrelatedOrMalformedResponses |
| Incomplete/unsupported upstream handshakes; valid JSON/SSE lifecycle | TestClientInitializeValidation, TestClientLifecycle |
| Legacy SSE June success and unsupported-version rejection | `core/internal/upstream/sse_test.go`: TestSSEClientAgainstLegacyFixture, TestLegacySSERejectsUnsupportedVersion |
| Pending/ready behavior, fallback, retained capabilities, missing/mismatched headers, unsolicited replies | `core/internal/virtual/handler_test.go`: TestInitializationLifecycleAndVersionMatrix |
| Migration upgrade/rollback preserving old sessions | `core/internal/store/migrations_test.go`: TestSessionLifecycleMigrationUpgradeAndRollback |
| Lifecycle persistence with warm caches on two nodes | `core/internal/e2e/e2e_test.go`: TestSessionResumesOnAnotherNode |

This matrix is a regression suite against the stated baseline, not independent interoperability certification. P02 and subsequent phases retain their own release gates.

### P01 verification result

On 10 October 2026, the complete `go test -race -count=1 -json ./...` run passed with **174 test/subtest pass events, zero failures and zero skips**, using isolated PostgreSQL 17 and Redis 7 containers. `go vet ./...` and `git diff --check` passed. The complete test event log is temporarily available at `/tmp/vmcp-p01-tests.jsonl`; it is not a durable release artifact. Temporary containers were removed after verification. No frontend code changed, so frontend checks were not rerun. P02 has not been started.
