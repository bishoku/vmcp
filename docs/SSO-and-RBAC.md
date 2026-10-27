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

| Role | Can |
|---|---|
| **viewer** | Read everything (`GET`) |
| **editor** | Change configuration, but not manage admin tokens |
| **admin** | Everything |

The highest matching role wins; users in none of the groups are refused. Admin API tokens map `read` to viewer and `admin`
to admin.

## Good to know

- Signed cookies are valid for **8 hours** and reference shared PostgreSQL session records.
  Logout revokes the session across control-plane replicas; operators can revoke all
  sessions for a subject. Database errors deny authentication. Role changes apply at
  the next sign-in; see [security operations](SECURITY_OPERATIONS.md).
- Cookie-authenticated writes must send the header `X-VMCP-CSRF: 1` (the UI does). Bearer-token calls do not need it.
- Tokens keep working with SSO enabled; use them for automation.

## Team ownership

OIDC groups can also map to [teams and projects](Teams-and-Projects.md). Team membership adds scoped management access without a local user directory. Global editor/viewer roles retain unmanaged-resource access; platform admins manage ownership mappings.
