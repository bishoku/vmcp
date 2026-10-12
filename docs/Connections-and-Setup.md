# Connections and guided setup

The **Connections** page visualizes saved access relationships:

**Agent keys / JWT identity bindings / anonymous access → virtual MCP server → upstream MCP servers.**

These are configuration relationships, not active clients or live traffic. A key
marked active is an unexpired, unrevoked credential; it does not indicate a live
session. JWT bindings remain conditional on verified claims, binding precedence
and IP restrictions. An anonymous virtual server bypasses credential authentication.
Disabled servers and revoked/expired credentials remain visible for investigation.

## Follow the setup guide

Open **Setup guide**, choose a saved virtual server if available, and follow:

1. Register upstream MCP servers with their transport, TLS and authentication.
2. Open each upstream and explicitly test its stored credentials. This tests the
   gateway-to-upstream hop; it does not verify a client's access to a virtual server.
3. Refresh upstream capabilities. Inspect stored tools, resources, resource
   templates and prompts. Reloading a discovery snapshot does not contact the
   physical upstream or perform fresh discovery.
4. Compose and save a virtual server. Select capabilities explicitly and review
   aliases, tool policies, authentication, PII rules and client feature permissions.
5. Grant an agent key or conditional JWT binding access. Newly issued secrets are
   shown once. Anonymous access requires no key and bypasses credential restrictions.
6. Open **Connect**, copy the authoritative endpoint and HTTP client settings,
   then replace the credential placeholder. Client configuration formats vary;
   adapt the settings to the client you use. Admin API tokens are not agent keys.
7. Explicitly verify discovery, then inspect **Activity** for recent stored events.

Configuration alone never marks a connectivity test or client verification as
complete. Viewers can inspect configuration and verify with a separate gateway
credential; editors perform configuration changes. Server authorization remains
independent of the UI role.

## Saved server views and drafts

Each saved virtual server has Overview, Capabilities, Access, Protection, Connect
and Activity views. URLs follow `/virtual-servers/<slug>/<view>`, for example
`/virtual-servers/team/connect`. The full-page editor is available to editors from **Edit configuration** at
`/virtual-servers/<slug>/edit` and marks saved version versus unsaved draft changes.
Both upstream and virtual-server lists open dedicated read-only detail pages.
Upstream pages switch to editing with **Edit upstream**; viewers inspect saved
configuration without edit controls. Navigating away from a dirty draft requires an explicit discard.

Topology node/relationship inspection opens capability aliases, policies, missing
objects and recorded upstream node health with timestamps. Configuration links
open the relevant upstream or virtual-server detail page, or select a key or
binding in its management editor.
Search relationships by name, capability, alias, policy or status, or select a
virtual server to focus the diagram. The accessible relationship table displays
50 rows at a time; **Show 50 more relationships** expands it. The diagram shows
at most eight edges on each side of one focused virtual server. This bounds visual
clutter; it is not an all-nodes force-layout graph.

Configuration reads are independent and cancellable. Refresh updates them;
there is no live subscription. Partial failures are shown and may leave older
successful data visible. The last-received timestamp is not an atomic snapshot
revision. Selected capability counts are not necessarily advertised/allowed
capability counts: missing objects, disabled upstreams, policy, negotiation and
client identity affect runtime behavior. Read-only tool policy relies on upstream
annotations.

## Deliberate browser verification

**Verify connection** uses the pinned MCP SDK in a lazily loaded browser module.
It creates a separate session, initializes with no sampling/roots/elicitation
permissions, pings, and lists only the feature families the server advertises.
It does not call tools, read resources, execute prompts, or submit client feature
responses. Results count the first page of each list and indicate further pages.
Success confirms discovery for that identity, not permission to execute every
listed capability or the absence of sensitive output.

For authenticated servers, enter an agent key or OAuth access token. It stays in
memory, is cleared from the input on submission, and is sent only to the configured
gateway. Verification never reuses the admin login token or cookies and refuses
redirects. Each request has a 10-second timeout; the overall operation is bounded
by 20 seconds and can be cancelled. Session DELETE is attempted on completion,
and the SDK transport is closed in all cases. Aborted/unreachable sessions may
remain until gateway expiry when cleanup cannot reach the server.

Configure `VITE_PUBLIC_GATEWAY_URL` at UI build time. Missing configuration or a
disabled virtual server blocks verification. An HTTPS UI also needs an HTTPS
browser-reachable gateway to avoid mixed-content blocking.

For cross-origin browser access, **both** layers must be configured:

- Set the data plane's `VMCP_ALLOWED_ORIGINS` to the exact UI origin. This is an
  ingress security check; the Go gateway does not itself emit CORS response headers.
- Configure the gateway reverse proxy for that same exact origin: answer OPTIONS
  preflight, allow POST/GET/DELETE and Authorization, Content-Type, Accept,
  MCP-Protocol-Version, MCP-Session-Id and Last-Event-ID headers, and expose
  MCP-Session-Id, MCP-Protocol-Version, WWW-Authenticate and Retry-After responses.
  Include `Vary: Origin`, including error responses. Do not use wildcard origins.

The demo's `benchmark/gateway.conf` includes this policy for
`http://localhost:3300` and `http://localhost:8800`, aligned with its data-plane
allowlist. Other deployments must configure their own exact origins, or serve
UI and gateway routes through one origin. Native MCP clients do not require CORS.

## Bounded observed activity

Activity reads at most 50 events for that virtual server from the last 15 minutes,
with a last-check timestamp and an explicit refresh button. It does not poll or
calculate session counts/traffic aggregates. No events does not prove no traffic;
audit storage/retention/failures can affect visibility. Admissions without outcomes
are not confirmed successes. **Open audit log** carries the virtual-server filter
into the full event list. Audit grouping and richer correlation inspection remain
in frontend phase F04.

## Verification scope

Unit/component regressions cover relationship semantics, inactive credentials,
anonymous access, alias/policy display, missing objects, partial failures, large
edge sets, saved views, setup steps, activity windows and SDK message sequences.
Chromium regressions cover keyboard inspection, configuration deep links, dirty
navigation and deliberate discovery using a separate gateway credential.
`python3 benchmark/test_gateway_cors.py` runs an isolated Nginx CORS regression
with Docker and removes its temporary container. It does not modify the demo stack.
These checks do not replace live-stack, responsive or accessibility testing.


## Audit investigation and responsive management

Audit Log initially groups **loaded events** by operation ID. Open Details to
inspect the raw admission/outcome metadata and copy operation, request or trace
IDs. Raw events retains a separate row per stored event. Shared request/trace IDs
alone do not establish an operation relationship. An admission without a loaded
outcome is labelled “outcome not loaded”: it can be pending, outside the selected
page/time/status filter, or missing from storage. It is not proof of failure.
Use the operation link to remove unrelated filters and load its stored events.
Confirmed outcome errors retain their recorded status and reason.

Filter by server, identity/key/subject, actor, operation, status and time. Copy
audit filter link preserves the supported filters for another authorized operator;
links do not grant access. Date/time inputs use the displayed browser timezone,
while copied links carry ISO timestamps. Refresh is manual. The last-loaded time
records the successful read, not a guarantee that all producers have flushed
buffered events. Retention and audit-mode behavior still apply on the backend.

On narrow screens the navigation scrolls horizontally and editor fields stack.
Wide audit and management tables scroll within their cards. Focus rings and native
dialog keyboard behavior support keyboard operation. Catalog search narrows
available resources/templates/prompts; it never removes saved selections. Test
and refresh an upstream after correcting URL/credentials or connectivity failures.

## Frontend verification

From the repository root:

```sh
npm --prefix web run test
npm --prefix web run lint
npm --prefix web run build
npm --prefix web run validate:api
npm --prefix web run test:browser
npm --prefix web run test:live
```

Install dependencies with `npm --prefix web ci` and install Chromium with
`cd web && npx playwright install chromium` before browser checks. Standard browser
tests mock API responses to exercise draft recovery, permissions, keyboard flows,
audit investigation and layouts at 375/768/1440px. They capture screenshots in
`web/test-results/` and check selected theme contrast pairs; they are not a complete
accessibility audit or pixel-diff suite.

`test:live` requires Python 3, Docker with Compose supporting `--wait` and JSON
configuration, and enough capacity to build/run the full demo. It resolves
`benchmark/compose.yaml` into a uniquely named project with new volumes, loopback
ports, network and build image tags. It uses development-only synthetic credentials.
The default isolated subnet is `172.30.73.0/24`; set `VMCP_UI_TEST_SUBNET` to another
unused private CIDR if needed. Do not run concurrent live tests on the same subnet.
The test seeds a real upstream, resource/prompt selection and key through the API,
then verifies login and MCP discovery from the built UI in Chromium. It exercises
both configured data planes behind the gateway, but does not assert every request
reaches a different replica or test SSO. Screenshots are in `web/test-results-live/`.
A finally block removes its containers, network, volumes and image tags, including
on failure; existing demo data and images are not used as test state. An externally
terminated Python process may require cleaning up the printed `vmcp-ui-*` project.
Rebuild your manual demo's web image to see the new UI; running it was not changed
by these tests.
