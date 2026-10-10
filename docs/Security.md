# Security

To report a vulnerability, follow [SECURITY.md](https://github.com/bishoku/vmcp/blob/main/SECURITY.md). This page is for operators.

## Secrets

| Secret | Protection |
|---|---|
| Upstream headers, mTLS client keys, OAuth client secrets | AES-256-GCM with `VMCP_MASTER_KEY`; write-only; never exported |
| Agent keys, admin tokens | Only hashes stored; shown once |
| `VMCP_ADMIN_TOKEN`, `VMCP_MASTER_KEY` | Environment; **replace the development defaults** |
| SSO sessions | Signed, stateless cookies (8h), CSRF header on writes |

Back up `VMCP_MASTER_KEY` separately from the database. Rotating it requires re-entering secrets (there is no automatic
re-encryption yet).

## Hardening checklist

- Terminate TLS in front of both services; restrict who can reach the control plane and UI.
- Set `VMCP_TRUSTED_PROXIES` correctly; use `ipAllowlist` on agent keys; give keys the minimum `virtualServers`.
- Use `deny`/`read_only` [policies](Tool-Policies.md) for destructive tools and [PII masking](PII-Masking.md) for sensitive data.
- Use per-upstream CAs rather than disabling verification (not possible anyway): [Upstream TLS](Upstream-TLS.md).
- Set rate limits and review the [audit log](Audit-Log.md).
- Prefer SSO + `read` scoped tokens; rotate and expire tokens.
- When enabling OAuth for agents, set `VMCP_AGENT_OIDC_AUDIENCE` and configure restrictive identity bindings.

## Trust model and known limits

- Upstream URLs remain privileged editor/admin configuration. Outbound MCP and token requests enforce operator-owned host/CIDR policy at connection time; private/loopback destinations need explicit authorization. Metadata destinations, redirects and environment proxies are blocked. Legacy SSE message endpoints must share the stream origin. See [MCP network security](MCP_NETWORK_SECURITY.md).
- Browser MCP clients need an approved `VMCP_ALLOWED_ORIGINS` entry. Native clients without Origin remain supported. Origin checks do not replace authentication or grant browser CORS access.
- Tool descriptions come from upstreams and are passed to models; a malicious or compromised upstream can try prompt
  injection through them. Use description overrides and policies to constrain what agents see.
- PII masking is pattern-based and best effort.
- SSO sessions cannot be revoked before they expire.
