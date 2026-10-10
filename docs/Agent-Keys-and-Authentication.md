# Agent keys and authentication

## Agent keys (data plane)

Agents authenticate to virtual servers with an **agent key**, sent as `Authorization: Bearer vmcp_...`.

```sh
curl -X POST -H "$A" -H 'Content-Type: application/json' $B/agent-keys -d '{
  "slug": "billing-bot",
  "description": "Nightly billing agent",
  "virtualServers": ["billing"],
  "ipAllowlist": ["10.20.0.0/16"],
  "rateLimitPerMinute": 60,
  "expiresAt": "2027-01-01T00:00:00Z"
}'
```

- The secret (`key`) is returned **once**, only a hash is stored. Lose it and you rotate.
- `virtualServers` lists where the key is valid. `ipAllowlist` (CIDRs) restricts source addresses; behind a reverse proxy
  configure `VMCP_TRUSTED_PROXIES` so the real client IP is used.
- `POST /agent-keys/{slug}/rotate` issues a new secret; `/revoke` disables the key immediately.
- A virtual server with `authRequired: false` accepts anonymous callers. Keep that for local use.
- Every call is attributed to its key in the [audit log](Audit-Log.md).

## Admin access (control plane)

| Credential | Role | Notes |
|---|---|---|
| `VMCP_ADMIN_TOKEN` | admin | Bootstrap token from the environment |
| Admin tokens (`/admin-tokens`) | `read` (viewer) or `admin` | Created in Settings or the API; shown once; rotate/revoke/expire like agent keys |
| SSO session | viewer / editor / admin | See [SSO and roles](SSO-and-RBAC.md) |

Send admin tokens as `Authorization: Bearer ...` to `/api/v1`. `GET /api/v1/me` tells you who the server thinks you are.
