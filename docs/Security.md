# Security

To report a vulnerability, follow [SECURITY.md](https://github.com/bishoku/vmcp/blob/main/SECURITY.md). This page is for operators.

## Secrets

| Secret | Protection |
|---|---|
| Upstream headers, mTLS client keys, OAuth client secrets | Versioned AES-256-GCM envelopes with entity/purpose/tenant AAD and key IDs; write-only; never exported |
| Agent keys, admin tokens | Only hashes stored; shown once |
| `VMCP_ADMIN_TOKEN`, `VMCP_MASTER_KEY` | Environment; **replace the development defaults** |
| SSO sessions | Signed cookies (8h) backed by revocable PostgreSQL sessions; CSRF header on writes |

Back up master/recovery keys separately from the database. `cmd/rotate-secrets` supports
restartable re-encryption with an old/new keyring. Deploy compatible readers first,
retain recovery keys and verify restoration before retiring old keys. Legacy ciphertext
lacks context binding until rotated. See [security operations](SECURITY_OPERATIONS.md).

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
- SSO logout revokes the shared browser session. Operators can revoke a subject's sessions; role changes apply at the next sign-in. Agent JWT containment uses an exact-token denylist, not universal IdP introspection.
- The optional approval hook gates request metadata/digests; it does not inspect returned prompt instructions. Treat upstream content as untrusted.

## Container images and release verification

Go services use statically linked binaries in a non-root Distroless Debian 13
runtime; the web image serves built assets with a non-root Nginx process. Builder
and runtime base images are pinned to multi-platform index digests. Weekly
Dependabot PRs maintain Dockerfile bases, Compose dependencies and GitHub Actions;
updates must pass CI and are not automatically merged. Go toolchain updates must
also keep `core/go.mod` and the Docker builder version aligned.

Every published application image is scanned in CI. Release candidates are pushed
by digest, then the exact candidate digest is scanned for both Linux AMD64 and
ARM64, signed with Cosign, verified against the publishing workflow's GitHub OIDC
identity and assigned its version tag. HIGH/CRITICAL findings, including findings
without a fix, block promotion. Scanner errors also fail the gate. JSON scan
reports are retained as workflow artifacts; SBOM and provenance remain attached
to the image. The GitHub release job independently checks all nine signatures
before creating release assets. Untagged candidate manifests can remain in the
registry after a failed scan and are not approved release images.

A scan is evidence for the vulnerability database at that time, not a permanent
security guarantee. Newly disclosed issues require rescanning and rebuilding even
when application code has not changed. Rebuild after approved base-digest updates;
changing a floating tag elsewhere cannot update a pinned image.

Helm defaults disable privilege escalation, drop Linux capabilities, use a
read-only root filesystem and RuntimeDefault seccomp, and disable service-account
token mounting. The local deployment Compose file applies the corresponding
application filesystem/capability restrictions with bounded writable temporary
mounts; its example credentials and plaintext database connections remain for
local development only. Stateful dependency containers retain their own startup
and storage requirements.

See [release signature verification](Release-Installation.md#verify-image-signatures)
for the operator verification command. Helm does not enforce registry signatures
without an external admission policy.

A newly uploaded signature can briefly be absent from registry referrer reads.
Publishing retries only Cosign exit code 10 with `no signatures found`, at most
ten attempts ten seconds apart. Wrong signer identity, certificate failures and
other verification errors fail immediately; the bounded retry never permits an
unsigned image to pass.
