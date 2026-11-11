# Integrating an existing enterprise approval service

Use this guide when an existing workflow cannot directly implement the
[v1 decision contract](../api/invocation-service.openapi.yaml). Authentication and
timeout options are described in [Invocation security](Invocation-Security.md).
The gateway protocol remains one POST endpoint per service, with `check` and
`consume` phases. This release does not add arbitrary API/JSON mapping, webhook
receivers, workflow orchestration, mTLS client certificates or provider-specific connectors.

## Responsibilities

```text
MCP client → vMCP → adapter decision endpoint → existing approval/workflow APIs
                       ↑
                 authenticated human review / trusted workflow callback
```

The adapter translates workflow states and owns durable correlation/consumption
state where the existing service lacks it. vMCP owns gateway dispatch reservation.
The business system owns business authorization and the operation itself. Neither
a gateway reservation nor an approval makes the business operation transactional.

Select `oauth_subject` when the client's verified vMCP token identifies the human.
Select `caller_context` when a shared application credential calls for different
customers. In the latter case the identity service verifies selected opaque session
references before attesting issuer, subject, ACR and authentication time. Never
trust caller-supplied customer IDs or MFA flags without independent verification.

## Map the lifecycle, not only JSON fields

| vMCP interaction | Adapter / existing workflow responsibility |
|---|---|
| First `check` | Authenticate vMCP; validate version, target and binding; create at most one durable workflow request for the operation. Return promptly. |
| Repeated `check` | Retrieve the same request. Return pending, step_up_required, denied or allowed; do not create another task. |
| Human review | Authenticate the reviewer independently; apply the organization's approver/delegation policy and display exactly the bound operation details. |
| Workflow callback | Authenticate and validate the callback, correlate it to the stored request, reject expired/mismatched updates. Never trust a client claiming approval. |
| `consume` | Revalidate identity/session, expiry, binding and required consent. Atomically change approved/unspent to consumed, then return allowed. |
| Duplicate `consume` | Reject; never grant a second execution. |

An existing `approved=true` flag is insufficient if it can authorize arbitrary
arguments or multiple executions. Add a durable compare-and-set consumption record
in the adapter when needed. Identity-only services (`requirements.approval=false`)
still receive check/consume, must revalidate the session and must reject duplicate
consumption; they need not ask a human for consent.

Use a durable unique operation key with tenant/application scoping. Bind it to the
application credential, verified issuer/subject/session, destination upstream/tool,
method, virtual server, policy/configuration revisions and gateway `sha256`.
Treat the digest as opaque: it includes execution context, and `details` is only
an allowlisted subset, so it cannot reconstruct the digest. Changes must not reuse
an old approval. Do not bind the literal `phase` into the immutable comparison.
Assurance may become stronger after step-up; compare stable identity and validate
the current assurance separately. Caller-context check initially lacks `user`;
consume contains the attested user and must match the verified session.

## HTTP examples

For static Bearer mode vMCP sends `Authorization: Bearer <service-token>`.
OAuth mode sends the acquired access token in the same header. Custom-header mode
sends the configured headers; explicit none mode sends no authentication headers.
All modes send `Content-Type: application/json`.

The [OpenAPI examples](../api/invocation-service.openapi.yaml) contain full check and
consume requests. The consume request has the same binding fields as the allowed
check and `phase: "consume"`. An approvalId is **not** added to the request: locate
the approval through operationId and its bound identity/context.

Pending review (HTTP 200):

```json
{"status":"pending","approvalId":"review-123","expiresAt":"2099-01-01T00:00:00Z","url":"https://approvals.example.com/review/123"}
```

Customer step-up (HTTP 200):

```json
{"status":"step_up_required","approvalId":"challenge-123","expiresAt":"2099-01-01T00:00:00Z","url":"https://approvals.example.com/verify/123"}
```

Allowed approval, for either an approved check or a successful consume (HTTP 200):

```json
{"status":"allowed","approvalId":"review-123","expiresAt":"2099-01-01T00:00:00Z"}
```

Allowed caller-context identity verification (HTTP 200):

```json
{"status":"allowed","approvalId":"attestation-123","expiresAt":"2099-01-01T00:00:00Z","verifiedUser":{"mode":"external","issuer":"https://idp.example.com","subject":"alice","acr":"2","authTime":1800000000}}
```

Denied, including an already-consumed approval (HTTP 200):

```json
{"status":"denied"}
```

Timestamps above are illustrative. Issue a short, appropriate expiry and use the
real user authentication time. A stale authTime cannot satisfy a freshness policy.
Responses may contain only documented fields. Do not add provider metadata, tokens
or internal error objects; keep those in the adapter. URLs must match the configured
HTTPS approval origin and contain no session credentials or fragments.

## Failure and recovery contract

| Situation | Required handling |
|---|---|
| Approval pending | Return immediately. Human waiting is outside the HTTP deadline. Client retains operationId and explicitly continues the same operation. |
| Check timeout / temporary outage | vMCP fails closed. Repeated checks must resolve to existing state; use bounded client backoff to avoid duplicate tasks/load. |
| Consume timeout / connection loss | Consumption may already have happened. Never automatically retry execution or clear the gateway reservation. Reconcile first. |
| Service A consumed, service B failed | No rollback across services is promised. Preserve consumed state and reconcile; do not assume approval is reusable. |
| Tool failed after consumption | Approval remains spent; inspect the business outcome before requesting a new operation. |
| Policy/arguments/user changed | Reject the old binding. A new operation requires the applicable new approval. |
| Adapter restart / concurrent replicas | Keep correlation and atomic consumption in shared durable storage; process memory is insufficient. |

The gateway only accepts HTTP 200 decision responses; HTTP 401/403/429/5xx are
service failures, not business decision formats. It does not automatically replay
requests, including after an OAuth 401. A later request can acquire a fresh token.
There is no gateway webhook API or automatic pending poller in this contract.
Arbitrary MCP clients may not understand vMCP's `-32004` decision UX. Integrate the
client flow as well as the server endpoint; never treat a browser callback as proof.

## Production checklist and acceptance scenarios

- Allowlist decision and token endpoints in vMCP egress policy. Provision least-privilege
  service credentials; use trusted TLS and rotate credentials through the service settings.
- Keep raw callerContext/session values out of logs and URLs. Persist only necessary
  protected state; correlate operational logs with opaque operation/approval IDs.
- Bound queue/state retention, monitor check/consume latency, pending age, token failures,
  denial reasons and ambiguous consumptions. Do not remove spent state while it is replayable.
- Test two concurrent consumes: exactly one may allow. Test repeated checks, process
  restart, expiry, changed user/arguments/policy and cross-tenant operation reuse.
- Test a lost consume response and partial multi-service consumption. Demonstrate that
  no retry silently executes the tool a second time.
- Test missing/weak/stale assurance and forged customer references. Verify both token
  and decision endpoint egress restrictions and authentication failures.
- Test the client's pending/step-up flow and preservation of operationId. Verify the
  eventual tool outcome separately from the approval decision.

The [security demo](Invocation-Security.md#quickstart-protocol-lab) and
[reference server](../core/internal/securitydemo/server.go) illustrate the protocol.
Their in-memory approval state is not a production adapter or a certification suite.
The schema/examples have automated repository validation; your adapter still needs
integration tests for the lifecycle and business-specific authorization above.
