# SSO and roles

The UI can sign users in through any OpenID Connect provider (authorization-code flow with PKCE and nonce).

## Enable it

Set on the **control plane**:

| Variable | Meaning |
|---|---|
| `VMCP_OIDC_ISSUER` | Issuer URL (must serve `/.well-known/openid-configuration`); enables SSO |
| `VMCP_OIDC_CLIENT_ID`, `VMCP_OIDC_CLIENT_SECRET` | Client registered at the provider |
| `VMCP_OIDC_REDIRECT_URL` | Public URL of `/auth/callback`, e.g. `https://vmcp.example.com/auth/callback` |
| `VMCP_OIDC_GROUPS_CLAIM` | ID-token claim with groups (default `groups`) |
| `VMCP_OIDC_ADMIN_GROUPS`, `VMCP_OIDC_EDITOR_GROUPS`, `VMCP_OIDC_VIEWER_GROUPS` | Comma-separated group names per role |

Register `VMCP_OIDC_REDIRECT_URL` as an allowed redirect URI at the provider and make sure the UI's host proxies `/auth`
to the control plane (the bundled nginx config does).

## Try it with Keycloak

The repository ships a ready-to-run Keycloak realm (development only):

```sh
docker compose -f deploy/docker-compose.yml -f deploy/docker-compose.sso.yml up -d --build
```

Open http://localhost:3000 and choose **Sign in with SSO**. Users (password = username): `alice` → admin, `bob` → editor,
`carol` → viewer, `dave` → no group (refused). Keycloak's own console is at http://localhost:8180 (`admin`/`admin`).

The realm (`deploy/keycloak/vmcp-realm.json`) defines the `vmcp` client (confidential, PKCE S256, redirect
`http://localhost:3000/auth/callback`), the groups `vmcp-admins|editors|viewers` and a *group membership* mapper that puts
`groups` (without the leading slash) into the ID token. Tokens carry issuer `http://localhost:8180/realms/vmcp` while the
control plane reaches Keycloak as `http://keycloak:8080` (`VMCP_OIDC_DISCOVERY_URL`).

For another provider, check that it puts the groups in the **ID token** and that the redirect URI and post-logout redirect
URI (the UI's origin) are registered.

## Signing out

Sign out clears the vMCP session and, when the provider advertises `end_session_endpoint`, sends the browser there so the
provider session ends too (otherwise the next click on SSO would sign you straight back in). Register the UI origin as an
allowed post-logout redirect URI.

## Roles

| Identity label | Authorization |
|---|---|
| **admin** | Explicit platform administrator authority, including bootstrap recovery |
| **viewer / editor / team / custom** | Only current role assignments and verified claim mappings grant permissions |

Authorization is always enforced. There are no legacy/shadow modes or activation settings.
Non-admin labels do not imply configuration reads, writes or audit access. Assign a built-in or custom role
to the issuer+subject, trusted OIDC mapping or named token ID through Access Management.

Admin API tokens retain the wire scope labels `read` and `admin`. A `read` token is an assignment-based
non-admin identity: it has no implicit GET permission and may receive explicitly scoped write permissions.
An `admin` token is a platform administrator. Token identity scope and granular permission assignments are distinct.

## Good to know

- Session cookies are valid for **8 hours** and contain only a signed reference (`vmcp_session_v2`). Identity and the
  required verified ID-token claim subset are encrypted in shared PostgreSQL session records. Migration 39 requires
  existing browser users to sign in again; old session data is not given an inferred issuer or claims.
- Mapping-derived permissions last at most **15 minutes**, capped by the verified ID token expiry. A newly required claim
  path needs a new sign-in. Direct subject assignments remain usable within the valid session lifetime. Platform role
  changes require sign-in; mapping/assignment edits affect the next protected request.
- Logout revokes access across replicas. Subject revocation is scoped to an exact issuer; the API defaults to the configured
  provider, and an explicit empty issuer selects legacy rows. DB or ciphertext integrity failures deny authentication.
  See [security operations](SECURITY_OPERATIONS.md).
- Cookie-authenticated writes must send the header `X-VMCP-CSRF: 1` (the UI does). Bearer-token calls do not need it.
- Tokens keep working with SSO enabled; use them for automation.

## Team ownership

OIDC groups can also map to [teams and projects](Teams-and-Projects.md). Team membership identifies ownership context without a local user directory. Scoped permissions still require explicit role assignments; neither team membership nor editor/viewer labels grant unmanaged-resource access.

## Verified claim mappings

Admins can manage typed claim/scope mappings through `/api/v1/identity-mappings` and role assignments through
`/api/v1/role-assignments`. Enabled OIDC mappings require the configured issuer exactly. Scope matching reads a string
claim in the verified ID token; it does not authenticate external OAuth access tokens as control-plane bearers.

`POST /api/v1/access/evaluate` remains a policy-only simulation. `GET /api/v1/access/status` reports session
capabilities, operational warnings and counts. Both audit and configuration authorization always apply;
neither endpoint can disable checks or create an authenticated session.

## Access management interface

Global administrators can open **Access → Access management** (`/access`) to manage roles, scoped assignments and typed
OIDC claim mappings. Builtin roles are readonly and can be copied into an independent explicit permission list. Updates
and deletes use the loaded version; conflicts require an explicit refresh rather than overwriting another administrator.

For a request-only audit reviewer, select `audit.events.read` and `audit.payloads.request.read` in a custom role, then
assign the principal/mapping, resource scope and required capture scopes explicitly. `audit.payloads.response.read` is a
separate permission. Resource ownership periods preserve historical access across transfers; selecting another period
is an intentional new grant. Legacy records still require their explicit tenant-wide legacy permissions.

The **Simulation** section accepts sample claim JSON and a permission/resource or persisted audit event ID. It shows a
policy-only result and never downloads audit messages, creates a session or applies a role assignment. Samples are not
stored in browser storage or URLs. The **Migration status** section reports operational warnings and counts at `/api/v1/access/status`; it cannot alter
permissions or backfill historical ownership.

Only the global admin role opens Access Management. Assigning a custom role containing `access.*` does not
delegate that administration. Both configuration and audit permissions always apply.

All sessions can open their permitted configuration screens. Navigation, direct resource routes, actions and
protected fields use self-only `POST /api/v1/configuration-access` decisions. The browser checks each configuration
read and proposed write before sending its data request; the actual API independently checks again.
Denied related inventories are not downloaded. Decisions are advisory and are never stored as authority in browser
storage. Configuration responses use `no-store`; capability request bodies are excluded from audit payload capture.

Existing upstream/virtual-server and credential edits send only changed fields. Creating a resource omits optional
fields the user cannot set and relies on server defaults; virtual-server creation uses POST rather than replacement.
Destination selection needs visibility of the relevant teams/projects/virtual servers as well as the applicable create
or access permission. Changing ownership or widening a credential still checks every original and proposed domain.
Export/apply, audit settings, token actions, tool-grant decisions and REST/mock actions retain their separate checks.
Audit metadata and request/response payload permissions remain independent.

A denied operation after a previously downloaded resource, sign-out, session expiry or claim expiry clears the mounted
configuration view and its drafts before rechecking access. Direct subject grants can continue after claim freshness
expires. An already sent operation is never replayed automatically. Review real IdP mappings, automation roles and
recovery access before publishing or updating the deployment.

For resource-local audit rule management, resource pages use their own audit-settings endpoint.
audit.settings.read controls reading local rules; audit.redaction.local.update controls saving;
audit.redaction.preview plus settings.read controls synthetic preview. Global settings remain tenant scoped.

Migration 40 removes both authorization mode selectors and their trigger. Policy revision advances to invalidate
in-flight decisions. Fresh installs apply it automatically. There is no migration/activation API or mode field on
`/me`, capability decisions, simulation or access status. Historical audit provenance classification is a separate
concept and remains protected.


### Builtin workflow dependencies

Virtual Server Editor includes scoped project reads for its project selector;
Credential Manager includes scoped virtual server reads for target selection.
These dependencies follow the assignment scope and do not grant cross-team reads
or permission to change publication, ownership or target configuration.
Automation Deployer supports tenant-wide export and resource-document deployment.
A full exported document may require additional governance, binding and audit
settings write roles to restore; prune and ownership transfer remain separate.
Migration 41 updates builtin templates; existing custom copies are unchanged.

A valid OIDC session labelled `team` can use independent subject assignments after
team membership changes. Expired group/claim data grants nothing; direct grants
remain valid until revoked or the identity session expires. Team ownership does
not implicitly grant or revoke explicit claim mappings.

Keycloak quickstart explicitly maps team groups to team-scoped console roles and
the auditor group to audit metadata only. Runtime identity bindings are separate.
Keep the installer ledger to preserve deliberate demo access removals on reruns.


## Platform administrators and operator roles

Platform administrator identity is established only by the deployment bootstrap
credential (`VMCP_ADMIN_TOKEN`), a named control-plane token with identity scope
`admin`, or an authenticated OIDC session admitted through the configured admin
groups. These administrators manage roles, assignments and mappings, and have
full configuration and audit request/response access. Admin group changes require
session revocation and a new sign-in to affect existing administrator sessions.

The builtin **Platform Operator** (`platform-operator`) is a broad permission
bundle, not an administrator identity. It includes sensitive configuration and
audit payload permissions subject to its assignment scope and explicit capture
allowlist. Assign it deliberately. It cannot manage Access Management or issue or
rotate administrator credentials. Custom roles containing `access.*` also cannot
establish administrator identity; `access.mappings.manage` can independently be
required for authorized team issuer/group configuration changes.

Migration 42 renames the old builtin `platform-admin` while preserving its UUID
and assignments, removes its `access.*` permissions, and increments its version.
Existing custom copies keep their names and permissions and do not become
administrators. Update scripts that address the old builtin slug; it has no API
alias. A conflicting custom `platform-operator` slug causes the migration to fail
transactionally rather than overwriting that role; resolve the custom role name
by creating a differently named copy and explicitly moving its assignments.

Operator assignments and mappings may all be disabled or removed. Their count
cannot prove that a working administrator login exists. Referenced roles/mappings,
ETags, trusted issuer checks and assignment scope checks remain enforced. The last
named administrator token may also be revoked: deployment bootstrap access is the
independent recovery root, not a database assignment counter. See the
[administrator recovery procedure](SECURITY_OPERATIONS.md#administrator-recovery).
