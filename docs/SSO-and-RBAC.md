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

## Roles

| Role | Can |
|---|---|
| **viewer** | Read everything (`GET`) |
| **editor** | Change configuration, but not manage admin tokens |
| **admin** | Everything |

The highest matching role wins; users in none of the groups are refused. Admin API tokens map `read` to viewer and `admin`
to admin.

## Good to know

- Sessions are signed, stateless cookies valid for **8 hours**. They cannot be revoked earlier, and role changes apply at
  the next sign-in.
- Cookie-authenticated writes must send the header `X-VMCP-CSRF: 1` (the UI does). Bearer-token calls do not need it.
- Tokens keep working with SSO enabled; use them for automation.
