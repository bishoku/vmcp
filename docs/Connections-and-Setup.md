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
6. Use the guide’s **Connect a client** section (also available in **Connect**),
   copy the authoritative endpoint and HTTP client settings,
   then replace the credential placeholder. Client configuration formats vary;
   adapt the settings to the client you use. Admin API tokens are not agent keys.
7. Explicitly verify discovery, then inspect **Activity** for recent stored events.

The guide shows a saved configuration summary and seven numbered steps alongside
the referenced upstream path. The selected server is retained in the URL. Endpoint
copying, client settings and explicit verification are available directly in the
guide. Changing server or saved version clears the verification credential and
results; referenced upstreams missing from loaded inventory remain visible.

Configuration alone never marks a connectivity test or client verification as
complete. Viewers can inspect configuration and verify with a separate gateway
credential; editors perform configuration changes. Server authorization remains
independent of the UI role.

## Server inventories

Upstreams and Virtual Servers use the same summary tiles, filter panels and server
cards as their saved detail pages. Counts cover the loaded inventory, independent
of filters. Search and configuration filters combine; clear filters restores the
inventory. Results are paginated in groups of 24, with filters returning to the
first page. Server names and View server links open saved details.

Upstream filters cover enabled state, transport and discovery snapshot state.
Recorded health is displayed separately from discovery: a saved snapshot never
implies a healthy upstream. Refresh list reloads stored data; Refresh capabilities
explicitly performs discovery and retains warnings for orphaned selections.
Virtual-server filters cover enabled state and authentication requirements.
Viewer roles retain inspection access; mutations require an editor role, and
version checks and deletion confirmations remain in place.

## Saved server views and drafts

Each saved virtual server has Overview, Capabilities, Access, Protection, Connect
and Activity views. URLs follow `/virtual-servers/<slug>/<view>`, for example
`/virtual-servers/team/connect`. The full-page editor is available to editors from **Edit configuration** at
`/virtual-servers/<slug>/edit` and marks saved version versus unsaved draft changes.
Both upstream and virtual-server lists open dedicated read-only detail pages.
Creation and editing use the same section panels as saved details. A sticky Save /
Cancel bar keeps draft state and actions visible while scrolling. Virtual-server
tool selection supports an upstream filter and name/description search; catalog
choices support search and kind filters without changing existing selections.
Aliases, description overrides, per-tool policies and keyboard reordering remain
part of the draft. Duplicate tool names block Save. Upstream headers must be added
or cleared before saving, and stored secrets stay unchanged unless replaced.
Upstream details share the virtual-server summary tiles and section panels for
connection settings, explicit verification, recorded node health, TLS, OAuth and
masked request headers. The discovery table supports family filters, search and
incremental display of fifty stored objects. Reloading those snapshots neither
tests connectivity nor changes an open configuration draft.
Upstream pages switch to editing with **Edit upstream**; viewers inspect saved
configuration without edit controls. Navigating away from a dirty draft requires an explicit discard.

Topology node/relationship inspection opens capability aliases, policies, missing
objects and recorded upstream node health with timestamps. Configuration links
open the relevant upstream or virtual-server detail page, or select a key or
binding in its management editor.
The initial configuration flow represents all configured identities, virtual servers
and upstream servers, including disconnected nodes and disabled or missing references.
Ribbon width represents configured tool selections; identity grants carry their
virtual server's tool count. These counts repeat across grants and are not runtime
metrics or proof of effective permission. Zero-tool relationships (including
catalog-only connections) remain visible as thin dashed lines.

The Connections page uses summary tiles for all loaded identities, virtual servers,
upstreams and configured relationships. Explore server jumps directly into a
Sankey scope; breadcrumbs and Back one level preserve drill-down navigation.
Expand canvas provides a taller scrollable diagram for larger inventories.
Relationship-index search, virtual-server and kind filters remain independent of
the diagram scope. Focused upstream routes can open a wide connection-detail view
from the Sankey, with searchable capability mappings, kind filters and stored
upstream observations. Closing details preserves the current diagram scope.

Hover or keyboard-focus a node to preview its paths; select it to pin the highlight.
Select a virtual or upstream server to unfold exposed aliases and upstream tool
definitions as two additional Sankey columns. Search narrows tool nodes directly
in the canvas; tool mappings do not open a separate table. Select
an upstream tool to trace every exact mapping back to identities, or select an
exposed alias to restrict the trace to that virtual server. Denied, conditional and
missing mappings remain visible. Unselected upstream tool definitions are available
from the upstream configuration page.

Columns with more than twelve nodes are bundled into deterministic, labelled groups;
all nodes and edges are represented in those groups. Expand a group or select
**Focus here** to drill into its connected neighborhood. Breadcrumbs and **Reset to
overview** restore broader context. Large tool columns use the same expandable groups, with back navigation and
breadcrumbs inside the diagram.
The diagram scrolls within its canvas on narrow screens and tall inventories.

Virtual-server Overview uses configuration summary cards, upstream composition,
access/protection controls and a gateway endpoint card. Counts and status describe
saved configuration; no runtime requests or activity metrics are inferred.

Search relationships by name, capability, alias, policy or status, or select a
virtual server to filter the relationship table independently of the overview.
The accessible table displays fifty rows at a time; **Show 50 more relationships**
expands it. Relationship inspection retains resource, template and prompt details.

Configuration reads are independent and cancellable. Refresh updates them;
there is no live subscription. Partial failures are shown and may leave older
successful data visible. The last-received timestamp is not an atomic snapshot
revision. Selected capability counts are not necessarily advertised/allowed
capability counts: missing objects, disabled upstreams, policy, negotiation and
client identity affect runtime behavior. Read-only tool policy relies on upstream
annotations.

## Credential and identity management

Agent Keys and Identities share the server detail design: summary tiles, searchable
and status-filtered tables, and fifty-row incremental display. Selecting a saved
record opens read-only inspection; authorized editors choose **Edit key** or
**Edit binding** to change it. Viewers can inspect grants and restrictions.

Editors use separate panels for identity details, searchable virtual-server grants,
network/rate limits, credential expiry and JWT conditions. Filtering server choices
preserves selections, including references absent from the loaded server inventory.
Dirty-draft guards and stale-version recovery remain explicit. Unrelated key edits
preserve the exact saved expiry instant. Exists conditions send no value.

New and rotated secrets retain the one-time acknowledgement dialog. Claim
evaluation uses supplied claims against saved bindings; it does not verify a JWT.
Changing evaluation inputs clears the previous result.

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

Configure `runtime.publicGatewayURL` in Helm, or `VITE_PUBLIC_GATEWAY_URL` at UI build time for development. Missing configuration or a
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

All virtual-server detail tabs share summary tiles and section cards. Capabilities
supports search, kind filters and incremental display of fifty selections, with
expandable recorded upstream observations. Access lists saved grants with searchable
states and expandable restrictions; its authentication panel explains conditional
JWT matching and anonymous credential bypass. Connect separates the gateway endpoint,
copyable client configuration and explicit discovery verification into panels.

Protection follows the Overview card layout: saved-policy summaries, tool overrides,
data masking and individual client feature permissions. Empty or omitted detector
lists mean all built-in detectors; omitted custom rules are shown as none.

## Bounded observed activity

Activity separates the selected time window, loaded-record count and last successful
check from the event table. Empty and failed reads have explicit states; a failed
refresh preserves earlier records and marks them as retained data. Operation links
open audit investigation, and a continuation link appears when more records exist.

Activity reads at most 50 events for that virtual server from the last 15 minutes,
with a last-check timestamp and an explicit refresh button. It does not poll or
calculate session counts/traffic aggregates. No events does not prove no traffic;
audit storage/retention/failures can affect visibility. Admissions without outcomes
are not confirmed successes. **Open audit log** carries the virtual-server filter
into the full event list. For correlation and raw event inspection, see the audit investigation section below.

## Audit investigation and responsive management

Audit Log uses the same summary tiles, filter panel and section cards as server
and identity management. Summary counts cover only loaded records and groups,
never total traffic or confirmed failures. The filter panel can be collapsed while
active filter chips remain visible; correlation filters include operation, request,
actor and stage. Individual chips and **Clear all filters** remove restrictions.
The wider detail drawer separates event context, exact operation correlation and
individual records, with expandable raw metadata and copyable IDs.

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

## Settings and administrator credentials

Settings shares the summary tiles and section panels used by server and identity
management. Gateway information and authenticated OpenAPI downloads sit alongside
searchable, scope/status-filtered token metadata. Selecting a token opens read-only
inspection; creation, rotation, revocation and deletion remain admin-only. Secrets
retain the one-time acknowledgement dialog. Expiry inputs use the displayed local
timezone.

Configuration as code separates the JSON source from the proposed change preview.
Viewers can export and inspect stored JSON; editors and admins can import, preview
and apply. Document or prune changes invalidate the preview. Applying uses the
preview base revision, and pruning retains explicit deletion confirmation. Export
excludes stored secrets and import preserves omitted credentials.
