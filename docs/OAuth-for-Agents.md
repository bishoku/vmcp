# OAuth for agents (data plane)

In addition to static [agent keys](Agent-Keys-and-Authentication.md), vMCP can authenticate agents using **JSON Web Tokens (JWTs) issued by an external Identity Provider** (Keycloak, Okta, Auth0, Entra ID, etc.).

Agents present standard IdP access tokens, and vMCP maps their claims to permissions through managed **identity bindings**. Core gateway functionality never requires an IdP; agent keys and OAuth JWTs can be used side by side.

---

## 1. Gateway configuration

To enable OAuth on the data plane, configure these environment variables on the `data-plane` service:

| Variable | Description | Default |
|---|---|---|
| `VMCP_AGENT_OIDC_ISSUER` | Expected `iss` claim in incoming JWTs (e.g. `https://auth.example.com/realms/prod`) | None (OAuth disabled) |
| `VMCP_AGENT_OIDC_DISCOVERY_URL` | OpenID Connect discovery base URL if different from issuer (split-horizon / Docker networks) | `VMCP_AGENT_OIDC_ISSUER` |
| `VMCP_AGENT_OIDC_AUDIENCE` | Expected `aud` claim in incoming JWTs (e.g. `vmcp-gateway`). **Mandatory** when OAuth is configured. | None |
| `VMCP_AGENT_OIDC_ALGS` | Allowed signing algorithms, comma-separated | `RS256,ES256` |
| `VMCP_PUBLIC_URL` | Canonical public base URL of the data plane (e.g. `https://gateway.example.com`). Used for RFC 9728 metadata. | Inferred from `X-Forwarded-*` / `Host` |

> [!IMPORTANT]
> Symmetric algorithms (`HS256`, etc.) and `none` are strictly rejected. The data plane only accepts asymmetric cryptographic signatures verified against the IdP's JWKS.

---

## 2. Identity bindings

Access to virtual servers is **denied by default**. An incoming valid JWT must match one or more **identity bindings** that authorize the requested virtual server.

### Conditions

Each binding defines 1 to 20 claim conditions that must **all** be satisfied (logical AND):

- `equals`: Claim value equals expected string (e.g. `azp` equals `billing-bot`).
- `contains`: Array claim contains string, or space-delimited scope string contains token (e.g. `scope` contains `mcp.read` or `roles` contains `agent`).
- `exists`: Claim key exists and is non-empty.

Claims can be queried by exact key or nested dotted path (e.g. `realm_access.roles`).

### Access resolution

1. vMCP finds all enabled bindings whose conditions match the token's claims.
2. If no binding matches: request is rejected with `HTTP 403 Forbidden` (`no identity binding matches this token`).
3. If no matching binding includes the requested virtual server: rejected with `HTTP 403 Forbidden` (`not authorized for this virtual server`).
4. If client IP does not match the binding's IP allowlist (when configured): rejected with `HTTP 403 Forbidden` (`client IP not allowed`).
5. The first matching binding by alphabetical slug order becomes the caller's principal: `oauth:<slug>`.
6. Any rate limit defined on that binding (`rateLimitPerMinute`) is enforced against the principal.

---

## 3. Managing bindings

Bindings are managed via the Web UI (**Identities** page) or the REST API:

```sh
# Create a binding for billing agents
curl -X POST -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' \
  http://localhost:8081/api/v1/identity-bindings -d '{
    "slug": "billing-service",
    "description": "Authorized billing service accounts",
    "conditions": [
      { "claim": "azp", "op": "equals", "value": "billing-bot" }
    ],
    "virtualServers": ["demo"],
    "ipAllowlist": ["10.0.0.0/8"],
    "rateLimitPerMinute": 120,
    "enabled": true
  }'
```

### Dry-run claim evaluation

To test which binding an incoming token will resolve to without making data plane calls:

```sh
curl -X POST -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' \
  http://localhost:8081/api/v1/identity-bindings/evaluate -d '{
    "claims": {
      "iss": "http://localhost:8180/realms/vmcp",
      "azp": "billing-bot",
      "roles": ["agent"]
    },
    "virtualServer": "demo",
    "clientIp": "10.1.2.3"
  }'
```

Response:
```json
{
  "decision": {
    "allowed": true,
    "principal": "oauth:billing-service"
  },
  "matches": [
    { "slug": "billing-service", "virtualServers": ["demo"] }
  ]
}
```

---

## 4. RFC 9728 Protected Resource Metadata

When OAuth is enabled on a virtual server, unauthenticated agents receive a `401 Unauthorized` response with a `WWW-Authenticate` header pointing to the RFC 9728 protected resource metadata:

```http
HTTP/1.1 401 Unauthorized
WWW-Authenticate: Bearer realm="vmcp", resource_metadata="https://gateway.example.com/.well-known/oauth-protected-resource/mcp/v1/virtual/demo"
```

Clients querying the metadata endpoint learn which authorization server issues tokens for this endpoint:

```sh
curl http://localhost:8080/.well-known/oauth-protected-resource/mcp/v1/virtual/demo
```
```json
{
  "resource": "https://gateway.example.com/mcp/v1/virtual/demo",
  "authorization_servers": ["http://localhost:8180/realms/vmcp"],
  "bearer_methods_supported": ["header"]
}
```

---

## 5. Audit log attribution

Every tool call through OAuth is recorded with full audit trail:
- `agentKey`: Principal identifier `oauth:<binding-slug>`
- `subject`: The caller's JWT `sub` (unique subject ID from the IdP)
- `clientIp`: The calling IP (respecting `VMCP_TRUSTED_PROXIES`)
- `status`: `ok`, `tool_error`, `error`, or `denied` (with denial reason)
