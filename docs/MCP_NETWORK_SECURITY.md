# MCP network security

vMCP enforces operator-controlled ingress and outbound network boundaries. These checks apply to MCP traffic and upstream OAuth token fetching; they do not provide process sandboxing, tenant isolation or prompt-injection protection.

## Ingress contract

All MCP POST, GET and DELETE routes validate supplied Origin and explicit MCP-Protocol-Version headers before authentication or body processing. `VMCP_ALLOWED_ORIGINS` is an exact comma-separated origin allowlist, for example `https://agent.example,https://console.example:8443`. Origins contain scheme and authority only; paths, credentials, `null`, malformed values and multiple Origin headers are rejected. Default ports and hostname case are normalized. An empty list denies all supplied browser Origins. Native clients may omit Origin. This is independent of CORS; no browser CORS permission is automatically added.

Only `2025-06-18` explicit protocol headers are accepted. Duplicate, empty or unsupported supplied headers return HTTP 400; a missing header uses the known session's version. An initialize request may still propose an unsupported version in its JSON body and receive the supported version, as defined by negotiation.

POST requires application/json content type (optional UTF-8 charset) and Accept containing both application/json and text/event-stream with nonzero quality. GET requires text/event-stream. Media failures return 415 or 406. GET opens an authenticated, session-scoped SSE stream for notifications and replay; see [delivery and reconnect behavior](MCP_DELIVERY.md). Authentication and virtual-server/session ownership checks apply to GET. POST bodies retain the 4 MiB limit. DELETE retains authenticated session ownership checks. Sessions are bound to the authenticated credential and verified issuer/subject where applicable; see [Authorization consistency](AUTHORIZATION_CONSISTENCY.md).

## Outbound policy

Both services use the same network policy for:

- Tool calls and session initialization/termination over Streamable HTTP or legacy SSE.
- Upstream discovery/refresh, connection tests and declarative apply preflight.
- Health probes and fallback URLs.
- Upstream client_credentials token requests and refreshes.

`VMCP_EGRESS_ALLOW_CIDRS` grants private/loopback destinations, such as `10.42.8.0/24,127.0.0.1/32`. The default is empty. `VMCP_EGRESS_ALLOW_HOSTS` optionally restricts all destinations to exact DNS names/IP literals; it has no wildcard syntax. Include both MCP hosts and token endpoint hosts when configured. A host grant alone does not authorize a private address: the resolved private IP must also match an allowed CIDR. With no host list, ordinary public unicast destinations remain allowed.

Example for an approved internal upstream and its internal token issuer:

```sh
VMCP_EGRESS_ALLOW_HOSTS=mcp.internal.example,idp.internal.example
VMCP_EGRESS_ALLOW_CIDRS=10.42.8.0/24,10.42.9.10/32
VMCP_ALLOWED_ORIGINS=https://agent.example
```

Configure the egress variables on **both** services and restart them after changes. They are operator settings, unavailable through the editor API. Invalid policy configuration fails service startup; clients constructed with invalid settings also fail closed.

The policy resolves the destination at every new dial, validates **all** DNS answers before dialing any, and connects directly to an approved IP. Mixed public/private answers fail unless every address is authorized. TLS continues to verify the original hostname or explicitly configured serverName; custom CAs and mTLS remain intact. Reused connections continue to the previously checked address. Changing DNS cannot redirect an existing TCP connection.

HTTP(S) URLs cannot include userinfo, fragments or IPv6 zones. Unspecified, multicast, link-local, reserved/documentation ranges and known metadata/platform endpoints are denied even under broad grants. IPv4-mapped addresses are checked as IPv4, and common IPv4 transition/translation ranges are denied to prevent bypasses. Private/loopback destinations need explicit grants; shared address space also requires a grant. Egress rules do not replace network-level firewalls, particularly with custom network routing or translation outside the process.

All HTTP redirects are rejected, including same-origin redirects. This prevents redirects from moving custom credential headers, OAuth tokens or mTLS to another origin. Use the final endpoint URL directly. Legacy SSE endpoint events must resolve to the same scheme, hostname and effective port as the stream URL; relative paths work. Cross-origin endpoints and credential-bearing endpoint URLs are rejected before any POST is sent. Duplicate endpoint advertisements cannot replace the first accepted endpoint. No cross-origin exception is provided.

Environment and custom HTTP proxies are disabled for this traffic. Otherwise a proxy could resolve or dial a different destination from the checked one. Transparent network routing remains an operator responsibility. Custom RoundTrippers supplied to the internal client constructors are rejected; the standard HTTP transport is cloned and secured while retaining TLS settings and pool reuse. Operator-configured SSO/agent JWKS/discovery, PostgreSQL and Redis connections are outside this MCP/token HTTP policy and retain their existing configuration paths.

## Development and deployment compatibility

Internal upstreams that previously worked without restrictions now need explicit grants. The Compose file passes the new policy variables to both services. To run the host fixture, first determine the actual address(es) of `host.docker.internal` as resolved **inside the containers**, grant those addresses as `/32` or `/128` (or an intentionally approved subnet), and recreate the services:

```sh
# Replace these examples with the addresses approved for your Docker environment.
export VMCP_EGRESS_ALLOW_CIDRS=192.168.65.254/32
export VMCP_EGRESS_ALLOW_HOSTS=host.docker.internal
docker compose -f deploy/docker-compose.yml up -d --force-recreate control-plane data-plane
```

The example IP is not portable; do not assume it is your Docker host. If both IPv4 and IPv6 answers exist, approve each legitimate address. Do not use `0.0.0.0/0` or `::/0` to make development setup convenient. Tests explicitly authorize only loopback fixtures in test processes; security tests reset that grant to verify denial defaults. Real deployments inherit no test grants.

A controlled restart is necessary when changing operator policy. Existing clients/token sources capture settings when constructed; this change does not implement dynamic policy revocation of open sessions/connections. For credential/configuration invalidation and authorization freshness, see [Authorization consistency](AUTHORIZATION_CONSISTENCY.md).
