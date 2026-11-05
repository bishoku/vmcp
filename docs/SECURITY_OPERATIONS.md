# Security operations

## Encryption upgrades and recovery

`VMCP_MASTER_KEY` remains a base64 encoded 32-byte primary key.
`VMCP_KEY_ID` defaults to `primary`. `VMCP_DECRYPT_KEYS` is a JSON object
of old key IDs to base64 encoded keys (maximum 16 keys total). Inject it as
secret material; do not put it in Git, command arguments or logs.
New AES-256-GCM envelopes authenticate their version, key ID, immutable row ID
and field purpose. Renaming an upstream preserves decryption. Legacy ciphertext
has no context authentication; it remains readable only to support migration.

1. Back up PostgreSQL and recovery keys in separately controlled storage.
2. Deploy these readers to every plane with `VMCP_CIPHERTEXT_WRITE_VERSION=legacy`
   while older readers remain. Keep the existing primary key during this step.
3. After every reader supports envelopes, enable `v1` writes everywhere.
4. Distribute the old/new keyring everywhere before changing the primary ID/key.
5. Run `go run ./cmd/rotate-secrets` from `core/` with the database URL and keyring
   injected through the environment. Batches scan 100 rows; individual updates
   compare the old ciphertext to preserve concurrent changes. Committed rows are
   checkpoints: rerunning skips records already using the primary key.
6. Repeat after writers converge, verify no legacy/old-key records remain, and
   restore a backup with retained recovery keys before retiring any key.

Rotation covers upstream headers, client private keys, OAuth client secrets,
MCP events and pending client replies. Rolling back to an old binary after v1
writes requires restoring compatible readers or a separately verified data
rollback; old binaries cannot decrypt new envelopes. Rotating the master key
also invalidates browser cookie signatures because the signing key is derived
from it. Rotation does not make a compromised live gateway safe.

## Identity containment

Production browser cookies reference shared `browser_sessions`; every request
checks PostgreSQL without an authorization cache. Logout revokes that session
before clearing its cookie and returns an error if revocation cannot commit.
Operators may revoke an account with a parameterized update of `revoked_at`
where `subject` equals the verified account ID. Existing cookies without a
session ID require sign-in after upgrade. Group/role changes require session
revocation and fresh sign-in. Identity-provider logout alone is insufficient.

Production JWT verification checks `revoked_jwts` on each authentication, using
SHA-256 of the exact encoded token. An authorized incident operator inserts the
32-byte digest and the token's verified expiry. Store failures deny access;
there is no propagation cache. Remove entries only after token expiry. This
mechanism neither introspects arbitrary JWTs nor replaces issuer, JWKS,
signature, audience, expiry and not-before validation. Active physical resource
subscriptions must also be terminated/revoked through their gateway session or
binding; the denylist affects subsequent bearer authentication.

## Privacy and approvals

With PII masking enabled for the respective direction, string values are masked;
sensitive object keys and numeric values are denied rather than renamed or
changing JSON types. Numeric exponent expansion is bounded. Image/audio blocks
and blob resources are denied as uninspectable. Limits: 10 MiB document,
100,000 nodes, depth 64, 1 MiB per string/key and bounded numeric encodings.
Tool JSON-RPC error messages and data follow the same policy. Inspection failures
never echo rejected data. Regex detectors can miss personal information and do
not establish data classification or semantic prompt-injection protection.

Set `VMCP_APPROVAL_URL` to an operator-owned HTTPS policy service to require
approval for every tool and catalog invocation. Requests carry actor, virtual
server, method, object, configuration revision and SHA-256 of content; they never
include raw arguments/content. Only HTTP 200 with exactly `{"allow":true}` permits
execution. Timeout (2 seconds), malformed responses, denied egress and outages
deny execution. Normal gateway SSRF restrictions apply. The external service
owns human approval, policy and digest attestations. Prompt selection is checked
before retrieval; raw prompt content still requires host-side trust boundaries
and review. Description overrides and masking are not injection defenses.

The current SQL containment workflow requires restricted incident-operator
access and an externally recorded change ticket; it is not a self-service
revocation API. Retention of expired identity records is an operator task until
the retention phase integrates their cleanup.


## Administrator recovery

Recovery authority belongs to the team controlling the deployment and its secret
store. `VMCP_ADMIN_TOKEN` is required at control-plane startup. Keep its value in
an access-controlled secret manager and verify that the on-call deployment
operators can replace it. It is a full administrator credential, including audit
request/response payload access. Granular roles and claim mappings cannot replace
or revoke it. It is not a runtime MCP credential.

A recovery drill must establish a working management login; a positive access
simulation or a nonzero assignment count is insufficient.

1. Obtain deployment and secret-store access. Preserve the database,
   `VMCP_MASTER_KEY` and decryption keyring; recovery does not require deleting
   assignments, editing authorization tables or resetting encrypted data.
2. Generate a new high-entropy bootstrap credential in the secret-management
   workflow and update the control-plane source of `VMCP_ADMIN_TOKEN`. For Helm,
   this is `credentials.adminToken` and its configured secret reference. Keep
   credential values out of shell arguments, tickets, repository files and logs.
3. Restart/redeploy **every control-plane replica** with the new secret. Environment
   changes do not hot-reload. Until replacement is complete, old replicas may
   still accept the previous bootstrap credential. Do not rotate the master key
   merely to replace this credential.
4. If invalid OIDC discovery/configuration prevents control-plane startup,
   temporarily remove `VMCP_OIDC_ISSUER` from the deployment and restart in token-only
   mode. Bootstrap is independent of IdP login, but cannot rescue a process that
   never starts. Fix deployment OIDC settings through the deployment workflow;
   Access Management edits claim mappings, not the configured issuer/admin groups.
5. Connect to the protected control-plane management API with the replacement
   bearer credential. Verify `/api/v1/me` reports `role: admin` and
   `via: bootstrap-token`, then verify `/api/v1/roles` can be read. Use tooling that
   injects the credential without exposing it in command history or output.
6. Repair explicit permissions/mappings as needed, or issue a named `admin` token
   for normal management. Restore OIDC deployment settings and prove normal admin
   access from a fresh session. Revoke obsolete sessions and compromised named
   tokens; replacing bootstrap does not revoke either automatically.
7. Review the recovery operations in audit records. Replace the temporary recovery
   credential again with the retained recovery secret and redeploy all replicas;
   verify the temporary credential is rejected. The startup contract requires a
   bootstrap credential, so do not leave it blank after recovery.

Bootstrap requests still require a healthy policy database and the normal audit
admission/durability rules. Recovery does not bypass authorization loading,
resource validation, tenant boundaries or strict audit failures. Repair a database
or audit dependency through its operational procedure first. Cookie-authenticated
administrators still require CSRF protection. Admin credentials themselves are
not recorded as audit payloads; actions retain normal actor/operation metadata.

Acceptance includes token-only recovery with no assignments or usable named admin,
old bootstrap rejection after replacement, immediate named-token revocation,
OIDC administrator access with no assignments, and operator/custom-role rejection
from management APIs. A real multi-replica/IdP deployment drill remains an operator
acceptance step; repository tests do not certify secret-store availability.
