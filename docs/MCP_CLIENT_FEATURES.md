# Negotiated client features (P10)

The gateway's supported protocol revision remains **2025-06-18**. This phase
adds client request routing inside admitted tool/resource/prompt operations;
it does not advertise later revisions or URL elicitation.

| Primitive | Downstream client requirement | Operator permission | Behavior |
| --- | --- | --- | --- |
| sampling/createMessage | sampling capability | clientFeatures.sampling | Routed to the host; no gateway model credentials |
| roots/list | roots capability | clientFeatures.roots | Host file:// identifiers; no gateway filesystem reads |
| notifications/roots/list_changed | roots.listChanged=true | clientFeatures.roots | Shared revision, forwarded to active/next-used physical sessions |
| elicitation/create | elicitation capability | clientFeatures.elicitation | June flat primitive forms; accept/decline/cancel retained |
| logging/setLevel | logging server capability | Available with durable MCP delivery | Shared session threshold; configured on supported physical sessions |
| notifications/message | logging server capability | Available with durable MCP delivery | Valid severity and bounded static data; original payload omitted |
| completion/complete | selected prompt/template | Upstream completion capability | P09's curated routing |

Implementation: `config/client_features.go`, `virtual/client_features.go`,
`virtual/roots.go`, `virtual/streams.go` and `upstream/duplex.go`. Migration 21
stores virtual-server permissions, shared logging thresholds, root-change
revisions and rich tool definitions. The management editor and API/GitOps
include the permission block. All three permissions default to false:

```json
{"clientFeatures":{"sampling":false,"roots":false,"elicitation":false}}
```

The physical initialization capability set is the intersection of operator
permissions and the downstream client's offered capabilities. Unsupported
extensions are omitted. Capability changes retire cached physical sessions on
next use; existing leases still complete under the authorization guard. During
physical initialization, client feature requests are rejected rather than
allowing operations before the handshake completes.

Client requests receive opaque mapped IDs, session-scoped durable routing,
metadata audit events and a 30-second reply deadline. Upstream-origin metadata
identifies the physical upstream without supplying its credentials. Responses
can arrive through another replica. Expiry, cancellation, stale authorization,
unknown IDs and duplicate replies fail closed. The host remains responsible for
showing the request and obtaining user approval. The gateway does not approve
sampling or elicitation or fabricate a host response.

Each admitted parent operation permits at most eight client requests and 8192
sampling tokens total, with at most 4096 requested output tokens per sample.
Requests are limited to 64KiB; client results to 1MiB. Sampling context aggregation
across servers is denied: includeContext must be absent or none. Forms permit
at most 32 primitive properties and reject external/composite schemas and
unsupported modes/formats. Accepted form content is checked for required/unknown
properties, primitive types, enum membership and numeric/string bounds. Host UI
validation remains responsible for the presentation and consent boundary.
Elicitation must not request sensitive information; enabling the feature requires
operator review of the selected upstreams. Structural validation cannot infer
whether arbitrary natural-language requests solicit secrets.

Client request/result content passes through the virtual-server PII policy;
binary content fails closed when privacy inspection is enabled. Error data is
masked on client reply admission. Logger data is always replaced with a static
omission notice, avoiding raw secret-bearing messages in replay, audit or
operational logs. This deliberately limits log detail; use approved upstream
observability channels for detailed diagnostics.

Root-change notifications are persisted per session. Active operations poll the
revision while renewing their lease; idle cached physical sessions receive the
change on next use. Roots are context supplied by the host, not an authorization
sandbox. The gateway never interprets them as permission to access its own files.

Rich tool discovery retains title, outputSchema, _meta and extension fields
through PostgreSQL and curated tools/list. Gateway aliases/description overrides
retain precedence. tools/call result JSON already retains structuredContent and
_meta under existing response/privacy bounds.

Regression tests: `virtual/client_relay_test.go` exercises sampling, roots,
elicitation refusal, sanitized logging and cross-node replies/root notifications;
`virtual/client_features_test.go` covers negotiation, budgets and malformed
requests/results/forms; `mcp/tool_test.go` and `e2e/tool_metadata_test.go` verify
metadata preservation; `controlplane/client_features_test.go` verifies policy
ETags and GitOps retention. These are repository HTTP/SSE fixtures; they are not
an unspecified SDK certification or proof of prompt-injection prevention.
