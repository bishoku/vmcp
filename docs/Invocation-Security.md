# Invocation security

Invocation security is independent of runtime access bindings. A valid access
credential still needs the configured authentication assurance and transaction
approval before a protected operation can execute.

## Configuration

Open an upstream and select **Invocation security** to configure its default
policy and local services. Tool rows show the effective policy and allow a
stricter tool rule. In **Settings → Shared invocation services**, platform
operators manage services available to every upstream. The corresponding API
is `GET` or `PUT /api/v1/invocation-security/shared-services`.

For upstream automation, use `GET` or `PUT /api/v1/upstreams/{slug}/invocation-security`.
Grant `upstreams.invocation_security.read` and `upstreams.invocation_security.manage`
at the appropriate team or upstream scope. A scoped write sends `upstreamId`,
`revision` and `policy` from the upstream editor, and replaces only that upstream's
policy and local services. Read the new revision after a successful write.
New or changed tool rules must use a name from the saved upstream catalog.
Existing rules whose tools are no longer in the catalog can be removed.

A local service can only be used by its upstream, and its name must differ
from shared service names. Endpoint traffic still obeys the gateway's egress
policy. Service tokens are encrypted at rest, omitted from responses and
retained when omitted from a subsequent write. Changing an endpoint requires
an explicit token. Management responses include `hasToken`.

The existing tenant endpoint `/api/v1/invocation-security` remains available
for whole-document automation. It requires `invocation_security.read` or
`invocation_security.manage` and uses the tenant document revision. Local
services are nested under their upstream's `services` key. Services defined
in v2.0.1 remain shared after upgrade.

```json
{
  "revision": 1,
  "services": {
    "corporate": {
      "url": "https://approvals.example.com/check",
      "approvalOrigin": "https://approvals.example.com",
      "token": "replace-with-service-credential"
    }
  },
  "upstreams": {
    "<stable-upstream-uuid>": {
      "identity": {"source": "oauth_subject"},
      "stepUp": {"acceptedAcrValues": ["2"], "maxAgeSeconds": 300},
      "tools": {
        "delete_records": {
          "stepUp": {"acceptedAcrValues": ["2"], "maxAgeSeconds": 60},
          "approvalServices": ["corporate"],
          "shareFields": ["dataset", "count"]
        }
      }
    }
  }
}
```

Read the stable upstream ID from the scoped endpoint. Tools
use their **original upstream name**, not a virtual-server alias. Policies also
apply to Omni invocation. Upstream defaults apply to resource/prompt/completion
operations routed to that upstream; tool overrides apply only to tools. Local
catalog objects without an upstream do not inherit an upstream policy.

Approval services accumulate across inheritance. Accepted ACR sets intersect;
the lowest maximum age wins. An empty intersection is rejected at configuration
time. Tool rules cannot disable upstream requirements. Shared field allowlists
intersect when both levels provide one; absent allowlists share no cleartext
arguments. The hash always covers the complete execution arguments, after gateway
argument masking. Duplicate JSON keys are rejected and large integers retain
exact precision. Different numeric spellings may conservatively require new consent.

`VMCP_APPROVAL_URL` has been removed. Setting it causes data-plane startup to fail
with a migration message, rather than silently omitting security enforcement.
The general configuration export/apply document includes `invocationSecurity`
when configured, with upstream **slugs** for portability. Secrets are omitted;
provision them when applying to a new deployment. Omitting this section leaves
existing policies unchanged. Remove an upstream policy before deleting its
upstream, including when using forced deletion. Exporting configured policies requires
`invocation_security.read`; applying them requires `invocation_security.manage`.

## User identity selection

Gateway authentication always identifies the caller using its agent key or OAuth
credential. Existing access bindings and caller context forwarding keep their
original roles. Each protected upstream/tool policy explicitly selects where
its **user** comes from; vMCP never guesses from the credential type.

| `identity.source` | Application / gateway identity | Verified user and step-up |
|---|---|---|
| `oauth_subject` | Verified OAuth credential | The same token's issuer/subject, `acr` and `auth_time` |
| `caller_context` | Agent key or OAuth application credential | A configured service validates the application customer session and attests the user and assurance |

For example, a mobile chat backend uses a shared agent key, while Alice and Bob
have separate application sessions. Select:

```json
"identity": {
  "source": "caller_context",
  "service": "corporate",
  "fields": ["customerId", "sessionId"]
}
```

Enable the existing upstream `callerContext` configuration with these fields and
its metadata namespace. Only the selected fields reach the verification service.
They are untrusted input until the service verifies the session and customer
binding. The service can use its application backend/session store or an identity
provider; vMCP does not assume these fields are OAuth claims and never accepts a
caller-supplied `mfa: true` as proof. Raw session values should be opaque,
short-lived references and must not appear in audit logs or challenge URLs.

The service runs before every invocation covered by `caller_context`, including
operations without an additional step-up or approval requirement. It runs first,
then separate approval services receive the verified user without raw session
context. The same service may
also handle consent. `requirements.approval` tells each service whether consent
is required from it. User identity selection, step-up and consent are distinct
policy decisions.

Tools inherit their upstream identity policy and cannot replace its source,
service or field mapping. An upstream without an identity default may select a
source separately for each protected tool. Missing/invalid context never falls
back to OAuth identity, and insufficient OAuth assurance never falls back to
caller context. Anonymous callers are rejected for customer-session policies.

## Step-up

In `oauth_subject` mode:

The gateway checks verified access-token `acr` and `auth_time` on each request.
Accepted ACR values must have agreed semantics in the configured identity provider.
Missing, insufficient or expired assurance returns HTTP 401 with the RFC 9470
`insufficient_user_authentication`, `acr_values` and `max_age` challenge. A client
must perform Authorization Code + PKCE with those requirements and submit a new
access token. Refreshing a token does not reset authentication age. If assurance expires after
an SSE response starts, the MCP error carries the same requirements in its data;
clients must also handle that case. Static agent
keys and anonymous callers cannot satisfy `oauth_subject` policies.

The token remains audience-bound to vMCP. Neither it nor OTPs are sent to approval
services or upstream tools. The existing upstream client-credentials token is a
separate service credential.

In `caller_context` mode, the verification service returns `step_up_required`
with a trusted user-facing URL when the customer session needs stronger or newer
authentication. The application completes that flow and retries the same operation.
vMCP asks the service again; it never treats a browser callback as proof. The
service returns `verifiedUser` with `issuer`, `subject`, `acr` and `authTime`;
vMCP checks the configured assurance requirement against this attestation. The
application's gateway credential remains unchanged. The service revalidates the
session during final consumption; the upstream remains responsible for business
authorization and session checks at execution time.

## Approval and session verification service contract (version 1)

The gateway sends an authenticated HTTPS POST, with a two-second deadline and no
redirects. Existing egress controls apply. `caPem` can configure a private CA.

```json
{
  "version": "1",
  "phase": "check",
  "operationId": "6b3debc7-f21e-45ac-900d-9acc65efb9c5",
  "identitySource": "oauth_subject",
  "user": {"mode":"oauth","issuer":"https://idp.example.com","subject":"alice","acr":"2","authTime":1800000000},
  "requirements": {"approval":true},
  "application": {"mode":"oauth","credentialId":"binding-id","issuer":"https://idp.example.com","subject":"alice","acr":"2","authTime":1800000000},
  "tenant": "default",
  "upstreamId": "upstream-uuid",
  "tool": "delete_records",
  "method": "tools/call",
  "server": "operations",
  "policyRevision": 4,
  "configurationRevision": 42,
  "sha256": "<execution-argument-digest>",
  "details": {"dataset":"demo","count":5}
}
```

The client supplies a UUID at `_meta["io.vmcp/operationId"]`. It is a correlation
identifier, not permission. Keep it unchanged when continuing a pending operation.
Each approval must bind identity, tenant, destination, method, policy and configuration revisions and
argument digest (including forwarded caller context). The service must independently authenticate the human reviewing
its trusted page. An opened URL or a client-reported completion is not approval.

For `caller_context`, `application` describes the authenticated chat application;
`callerContext` contains only the selected fields. `user` is initially absent and
is filled from the identity service's attestation for subsequent services and
consumption. `requirements.stepUp`, when set, contains the effective
`acceptedAcrValues` and `maxAgeSeconds`. Bind the consent to both application and
verified user/session, not merely to the shared application credential.

HTTP 200 responses use this shape:

```json
{"status":"pending","approvalId":"consent-id","expiresAt":"2027-01-01T00:00:00Z","url":"https://approvals.example.com/review/consent-id"}
```

`status` is `pending`, `step_up_required`, `denied` or `allowed`. `approvalId` and a future `expiresAt`
are required except for denied responses. The caller context identity service
must include `verifiedUser` on `allowed`, for example
`{"mode":"external","issuer":"https://idp.example.com","subject":"alice","acr":"2","authTime":1800000000}`.
Check and consume must identify the same user. `url` is optional and must match the configured
HTTPS approval origin. Unknown fields, invalid responses, timeouts and outages
fail closed. Pending/denied results use MCP error `-32004`; pending data includes
`decision`, `operationId` and `notExecuted: true`. This is a vMCP extension, not
standard URL elicitation; arbitrary MCP clients may not implement its UX.

Once all checks allow, vMCP reserves the operation ID durably in PostgreSQL, then
sends the same request with `phase: "consume"` to each required service. Services
must atomically consume the consent and reject a second consumption. Only then
can execution proceed. A partial consumption or ambiguous dispatch is **not**
automatically retried. The operation remains reserved, even if execution later
fails; reconcile with the upstream before creating another operation. This
provides at-most-once gateway admission, not an atomic transaction with an
external business system. Audit payload capture redacts MCP `_meta`, `callerContext` and `sessionId` fields.
Dispatch reservations currently have no automatic
retention cleanup: do not delete them while old operation IDs can be replayed.

## Quickstart protocol lab

From the repository root:

```sh
./deploy/helm/vmcp/quickstart.sh --security-demo
```

Requires Docker Compose, Python 3 and OpenSSL. This option starts an **isolated
Compose stack**, not a Helm installation: PostgreSQL, Keycloak, both vMCP planes,
a sample approval service, sample MCP upstream and a browser client built from
this checkout. It does not use or change an existing cluster. Generated local
credentials/certificates live in `local/security-demo/` and are gitignored.

Open https://localhost:8844 and trust the generated development certificate.
Sign in as `alice` or `bob` with `demo-password`. OTP is enrolled when first
requested by the real Keycloak conditional level-of-authentication flow. Use an
authenticator app; no MFA claim is fabricated by the demo.

Choose **OAuth user** or **Application + caller context** before signing in:

- OAuth mode uses `security-demo` and the user's vMCP-audience token.
- Caller context mode uses `security-context`. The demo application backend holds
  one agent key for Alice and Bob; it does not expose that key to the browser.
  Separate customer OAuth login creates an opaque application session. Only
  customer/session context travels alongside the shared application credential.
  A distinct customer-token audience prevents using this token as a vMCP token.

Switch modes to exercise the same scenarios with both identity models. Sign in as
Alice then Bob to verify that the application stays the same while the verified
customer and approvals change. Initialize the connection and list tools. Scenarios:

| Tool | Requirement |
|---|---|
| `echo` | Normal authenticated access |
| `step_up` | ACR 2, authentication within 60 seconds |
| `delete_records` | Operation-specific approval |
| `export_records` | Step-up and approval |
| `execution_counts` | Inspect actual execution counts |

Tool names are exposed as `security-demo__<name>` or `security-context__<name>`. For approval: send a call,
open the trusted review link, authenticate as the same user through the separate approval OAuth client, review details and
approve/reject. Return to the client and retry the **same operation**. Changing
arguments must reject the old approval. New operation IDs require fresh consent.
In caller context mode, open **Verify this customer session** for step-up;
the customer completes Keycloak OTP and the external service updates the original
application session. The link contains a challenge reference, never the session
credential. Return and retry the same operation. Missing, expired or mismatched
sessions fail even when the application's agent key is valid.

After an OAuth redirect, initialize the MCP connection again; the argument draft
and operation ID are retained. The browser stores its short-lived access token
in session storage for this development exercise; it never prints the token.

The client also supports resource/prompt listing and calls, completion, and
interactive form elicitation/sampling responses from compatible upstreams. The
bundled backend supplies basic resources, prompts and completion; it does not
initiate sampling/elicitation. These client capabilities can be tested against
another configured upstream. URL elicitation is not advertised.

Approval state in this demo is in memory and lost on restart; execution counters
are also demonstration-only. Gateway dispatch reservations are persisted. The
sample approval service and public development credentials are not production
components.

```sh
# Stop the lab, preserving its gateway database.
docker compose -f local/security-demo/compose.json down
```

Protocol references: [RFC 9470](https://www.rfc-editor.org/rfc/rfc9470.html),
[Keycloak step-up authentication](https://www.keycloak.org/docs/latest/server_admin/#_step-up-flow).

To run the browser regression scenarios against a fresh lab (requires the web
workspace dependencies and Playwright Chromium):

```sh
VMCP_DEMO_OTP_FILE=/tmp/vmcp-demo-otp.json node web/scripts/test-security-demo.mjs
```

The test enrolls real OTP for the demo users. The optional file stores test-only
OTP seeds with mode 0600 so tests can run again; delete it when recreating the
Keycloak realm. This file is never needed by the demo application itself.

## Operator execution settings

The upstream detail page's **Invocation security** panel and tool rule editor also
manage execution settings. Existing `upstreams.invocation_security.read/manage`
permissions apply within the same resource scope. Runtime agents cannot change them.
The editor shows inherited values, effective behavior and affected catalog tools
before saving. Read-only access has no mutation controls; stale revisions retain
the draft and require explicit reconciliation.

- `retry`: `never` or `once`. Absent inherits the upstream value, then defaults to
  `never`. An explicit tool setting overrides the upstream default. Only eligible
  MCP calls can repeat once after a transport failure; REST and approval/session
  verification bound calls remain non-replayable. A hint from the upstream is not
  authorization. No idempotency-key guarantee is implied.
- `readOnly`: boolean operator classification for virtual-server `read_only`
  filters. Absent inherits, then defaults to false. An explicit tool false removes
  an inherited classification. This setting never enables retry by itself.

The existing upstream-scoped PUT document, global admin configuration and GitOps
`invocationSecurity.upstreams` section all carry these fields. Export uses upstream
slugs; persisted policies remain keyed by stable upstream IDs. No SQL migration is
needed for these additive encrypted JSON fields. An example GitOps fragment:

```yaml
invocationSecurity:
  upstreams:
    orders:
      retry: never
      readOnly: false
      tools:
        get_order_status:
          retry: once
          readOnly: true
        create_payment:
          retry: never
          readOnly: false
```

Identity, step-up and approval requirements still compose monotonically. Changing
retry/classification cannot remove an inherited identity or approval requirement.

## MCP argument validation

Virtual and Omni MCP dispatch validate the original tool InputSchema before
approval consumption and again after argument masking. Invalid calls are rejected
with generic invalid-params errors without argument values or schema diagnostics.
REST retains its existing validator and tool-error response contract.

Schemas must declare an object root and fit 64 KiB, 4,096 expanded nodes and depth
32. Acyclic local JSON-pointer `$ref` is supported; external, recursive and dynamic
references are rejected without network/filesystem resolution. Arguments are
bounded at 4 MiB, 100,000 nodes and depth 64. Invalid/unsupported schemas fail closed
at invocation. Snapshot-owned compiled validators and a bounded 256-entry shared
cache avoid per-call compilation. Upstream schema changes produce a new validator.
