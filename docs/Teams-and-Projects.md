# Teams, projects and upstream tool approval

Teams represent externally managed IdP groups. vMCP has no local user/password or
membership editor. Platform admins create teams with an exact OIDC issuer and
exact group claim value, then assign projects and upstreams to one team each.
Use stable group IDs when available; email group names are accepted when that is
what the trusted provider emits. User email is not evidence of group membership.
The existing `VMCP_OIDC_GROUPS_CLAIM` setting selects the claim (including dotted
paths). Missing claims never create teams or grant default editor access.

## Ownership

- An upstream has one owner team, or an empty `ownerTeam` for Default/unmanaged.
- A project has one owner team, or Default/unmanaged ownership.
- A virtual server has one `project` (default `default`) and inherits its owner.
- A project can contain multiple virtual servers. Upstreams are independent of projects.
- Only platform admins manage team mappings, projects, and ownership transfers.
- Team members manage their own connections, virtual servers and credentials.
  A credential spanning another team's virtual server cannot be managed by them.
- Existing global viewer/editor roles retain access to unmanaged resources.
  They do not override team ownership. Platform admins retain full management access.
- Team-only users can create owned upstreams and virtual servers in their projects;
  they cannot create unmanaged resources. API checks enforce this independently of UI.

Migration 33 assigns existing virtual servers to Default and leaves upstreams
unmanaged. Existing endpoint URLs, keys and tool selections are retained. Global
roles are not automatically granted to unmatched users. The built-in Default
project cannot be assigned to a team.

## Example

In **Teams & projects**, a platform admin registers `crm` with issuer
`https://idp.example` and group `crm-product-owners`, and `channels` with group
`chat-product-owners`. They create project `customer-chat` owned by `channels`.
The CRM upstream belongs to `crm`. A member of `channels` creates a virtual server
under `customer-chat`, selects `crm/get_customer`, and requests access from the
same management page. A CRM owner approves the request. Selecting a tool alone
does not authorize it. The tool must also pass the existing virtual-server policy
and caller authentication/quota checks.

Team membership comes from verified OIDC sign-in claims. Team claims expire after
15 minutes and require a fresh sign-in to renew. Existing global session roles
keep their original lifetime; expired team claims no longer add scoped rights.
An IdP must emit current claims on that fresh authentication. This is bounded
claim freshness, not instant LDAP change notification or SCIM synchronization.
Changing a team mapping is checked from PostgreSQL on subsequent API requests.
Existing pre-upgrade sessions have no team claims and must sign in again for team access.
Audit identifies people by issuer/subject; durable permissions belong to resources.

## Tool approvals

A grant identifies `(virtualServer, upstream, tool)` and has `pending`, `approved`,
`rejected` or `revoked` status. Requests are made by the virtual-server owner;
decisions are made by the upstream owner (or platform admin). Updates use the
record's `version`; stale changes fail with 412. This is permission to expose a
tool, not a human approval on each invocation. It does not enforce backend row-level
access to customer data.

Unmanaged upstreams do not require grants. Same-team tools are implicitly allowed
unless explicitly rejected/revoked; a same-team request creates an approved record.
A managed upstream still requires approval when consumed from Default. A decision
for one virtual server grants nothing to other servers, even in the same project.

The data plane filters tools when publishing its immutable configuration snapshot.
No ownership database lookup is added to each tool call. Existing snapshot freshness
and pre-dispatch reauthorization rules apply; revocation is not instantaneous and
cannot undo an external side effect already dispatched. It affects listing and calls
on existing sessions as snapshots converge.

Ownership transfers revoke explicit grants involving the changed source or consumer.
Previously implicit same-team access is recalculated from the new ownership. Review
and reapprove required connections after transfer. Returning an upstream to unmanaged
removes the approval requirement and is therefore platform-admin-only.

This release's approval workflow covers tools. Cross-team resources, prompts,
subscriptions and upstream instructions are not implicitly shared: they require an
unmanaged upstream or the same owner team. No semantic search or dynamic registry
is introduced. The shared tool picker exposes tool metadata, not connection secrets.

## API and GitOps

- `GET/POST /api/v1/teams`: list teams; platform-admin create/update.
- `GET/POST /api/v1/projects`: list visible projects; platform-admin create/update.
- `GET/POST /api/v1/tool-grants`: list visible requests; request/decide with `version`.
- `GET /api/v1/tool-catalog`: paged shared tool metadata for explicit selection.
- Upstream writes accept `ownerTeam`; an empty string selects unmanaged ownership.
- Virtual-server writes accept `project`; `default` selects the legacy workspace.
- Omitted ownership fields preserve existing assignments on update.

Tenant-wide export/apply requires platform admin once teams exist. With no teams,
legacy GitOps role behavior remains unchanged. Export includes teams, projects,
resource assignments and explicit grants. Apply reconciles these atomically;
omitting governance arrays preserves them. `prune` continues to manage upstreams,
virtual servers and bindings, not teams/projects/grants separately. There is no
project-scoped GitOps endpoint in this release. Use scoped resource APIs for team
changes. Exported identity mappings are configuration metadata; restrict their access.

Team/project records can be updated; deletion/archival UI is not included in this
first release. Neither a user directory nor IdP group enumeration is required.

## Bulk tool access workflow

Adding tools to a virtual server saves selections; it does not submit approval
requests. In the server's Tools editor, the approval summary distinguishes
**Request not submitted**, **Pending owner approval**, **Approved**, and
**Approval not required**. It counts approval-cleared selections, not the live
agent catalog: policy and upstream availability still apply.

For a saved server, choose **Request all missing approvals**, or select tools and
choose **Request access for selected**. Unsaved drafts must be saved first.
For more than 200 missing approvals, the action requests the next 200; refresh
and repeat for the remaining tools. Rejected/revoked tools are never silently included in the all-missing action;
explicitly select them to request again. Review the server/upstream groups and
supply a shared reason before submitting. Existing approvals remain unchanged.

In **Teams & projects → Tool access**, **Needs my review** shows authorized
pending requests. **My servers’ requests** shows outgoing requests for servers
you can manage; **All visible requests** includes previous decisions. Select
individual rows or a whole server/upstream group to approve, reject, or revoke.
The review shows the consumer project/team and upstream owner. Rejection and
revocation require a reason. Server-editor links open this section scoped to the
selected virtual server.

`POST /api/v1/tool-grants/bulk` accepts an `items` array of 1–200 grant objects.
Each item uses its current `version` (0 for a new request), passes the existing
ownership checks, and commits independently with a per-tool mutation audit.
The response reports `changed`, `unchanged`, `failed`, and an outcome/error code
for each identity. Duplicate identities invalidate the batch before any writes.
Refresh after conflicts or an interrupted response; never replay an entire batch
blindly. Successful approval means the control-plane saved the decision, not
that every data-plane instance has already published it.

## Composing tools in the virtual server editor

The Tool picker combines **Available** and **Shared** tabs. Available lists stored
catalogs of upstreams you can manage; Shared exposes metadata from other teams,
without connection details. Both tabs start without an upstream selection. Choose
an upstream, or search across the current tab without choosing one. **All
upstreams** starts with collapsed groups, with ten upstream groups shown at a time.

Each group can be collapsed and shows its tool/selection counts. Open a group to
browse 25 tools at a time, add an individual tool, or add all remaining tools from
that group. While searching, the bulk action adds only matching tools, including
matches beyond the currently displayed page. Existing selections are not added
again. These actions edit the draft only; saving and approval submission remain
separate.

On desktop the bounded picker sits next to the wider Exposed tools panel. On
smaller screens it starts collapsed under **Add tools**. Expanding catalogs does
not make the picker grow without limit; its results scroll independently.
