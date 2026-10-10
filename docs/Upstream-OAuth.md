# Upstream OAuth (Gateway to Upstream)

When an upstream MCP server requires OAuth 2.0 authentication, vMCP can act as an **OAuth client**, automatically obtaining and presenting `Bearer` access tokens using the **OAuth 2.0 Client Credentials Grant** (RFC 6749 §4.4).

---

## 1. Upstream configuration

OAuth settings are configured per upstream under the `auth` block in the REST API, declarative configuration, or the Web UI (**Upstreams** page):

```json
{
  "slug": "crm-service",
  "name": "CRM Service",
  "url": "https://crm.internal.example.com/mcp",
  "transport": "streamable_http",
  "auth": {
    "type": "oauth_client_credentials",
    "tokenUrl": "https://auth.internal.example.com/oauth/token",
    "clientId": "vmcp-gateway",
    "clientSecret": "super-secret-key",
    "scopes": ["crm.read", "crm.write"],
    "audience": "https://crm.internal.example.com"
  }
}
```

### Configuration fields

| Field | Type | Description |
|---|---|---|
| `type` | string | `oauth_client_credentials` or `none` (to disable OAuth) |
| `tokenUrl` | string | Absolute HTTP(S) URL of the authorization server's token endpoint |
| `clientId` | string | Client identifier issued by the identity provider |
| `clientSecret` | string | **Write-only.** Encrypted at rest using AES-256-GCM (`VMCP_SECRET_KEY`). Never returned in API responses or export files. |
| `scopes` | string[] | Optional array of OAuth scopes requested in `scope` parameter |
| `audience` | string | Optional `audience` parameter sent to token endpoint (standard for Auth0, Keycloak, etc.) |

> [!CAUTION]
> An upstream cannot have both OAuth client credentials and a static `Authorization` header configured. Attempting to save both produces an HTTP 422 validation error.

---

## 2. Token lifecycle and caching

The gateway manages token acquisition transparently:

1. **Process-wide cache**: All agent sessions and internal operations (tool discovery, health probes, connection tests) share one token cache per upstream configuration.
2. **Singleflight concurrency**: Multiple simultaneous requests wait on a single in-flight token fetch rather than hammering the identity provider.
3. **Proactive refresh**: Tokens are refreshed approximately 30–40 seconds before their `expires_in` deadline with randomized jitter.
4. **Reactive 401 retry**: If an upstream rejects a token with `HTTP 401 Unauthorized` (e.g. revoked token), vMCP invalidates the cached token, requests a fresh token, and replays the request once.

---

## 3. Resilience and error isolation

vMCP isolates identity provider failures from upstream server health:

- **Circuit breaker protection**: If the OAuth token endpoint is unreachable or returns HTTP errors, the call fails immediately with an explicit error. Crucially, **token acquisition failures do not count toward the upstream's circuit breaker**, ensuring temporary IdP outages do not mark healthy upstream services as broken.
- **Secret masking**: If an identity provider echoes the client secret in an error message, vMCP redacts it (`***`) before logging or returning error responses.

---

## 4. Declarative GitOps and Secrets

In [declarative exports](REST-API.md) (`GET /api/v1/export`), the `auth` block is exported without secrets:

```yaml
upstreams:
  - slug: crm-service
    name: CRM Service
    url: https://crm.internal.example.com/mcp
    transport: streamable_http
    auth:
      type: oauth_client_credentials
      tokenUrl: https://auth.internal.example.com/oauth/token
      clientId: vmcp-gateway
      scopes: [crm.read, crm.write]
      audience: https://crm.internal.example.com
```

When re-applying this document via `POST /api/v1/apply`, omitting `clientSecret` preserves the existing encrypted secret in the database.
