# Unreleased: prioritized readiness hardening

Tracking issue: #55. No release version, tag, deployment or merge is authorized
by this note. Existing release metadata is unchanged pending release planning.

## Behavior changes requiring upgrade preparation

- Automatic MCP transport retries are now **off by default**. Operator-owned
  upstream/tool invocation policies can explicitly select `retry: once`.
  Upstream annotations no longer enable retries. REST and approval/customer
  verification bound calls still never automatically replay.
- Virtual-server `read_only` filters now require an effective operator
  `readOnly: true` classification. Existing tools relying solely on upstream
  annotations become hidden until explicitly classified. Review and configure
  policies on every data-plane version before exposing traffic; drain mixed
  versions during the rollout so older nodes cannot retain hint-based behavior.
- MCP InputSchema is checked before dispatch and after argument masking. Invalid
  arguments previously accepted by tolerant upstreams are rejected. External,
  dynamic or recursive schema references and schemas exceeding documented
  bounds fail closed. Acyclic local JSON-pointer references are supported.
- Legacy ciphertext reading now requires explicit
  `VMCP_ALLOW_LEGACY_CIPHERTEXT=true`; unset means disabled. Inventory and rotate
  old records with the opt-in enabled, verify restore and then disable it. Old
  recovery keys must remain available for retained backups.
- Omni model endpoints now enforce egress policy. Grant approved private CIDRs
  before rollout. Environment proxies and redirects are disabled. Mounted CA
  files are supported without disabling TLS verification.

## Reliability and operations

Generation-bound half-open completion releases ignored probes and fences late
results. Critical workers trigger unready/graceful shutdown on panic rather than
silently continuing; pending side effects retain unknown audit outcomes.
Web responses include CSP, frame, content-type and referrer protections. The CSP
allows HTTP(S) connections for deployment-configured gateway origins; deployments
with fixed origins can narrow connect-src at their edge.

Startup log-level selection, trace-aware safe error logging, runtime/GC metrics,
renew outcomes and probe age improve diagnosis. Legacy admission-lock counters
remain zero after removal of the DB capacity gate.
Exporters can expose cached receipt/backlog/age metrics on a private optional
listener. No revocation cache, lease relaxation, try-lock-only capacity change or
stream pass-through is introduced.

New policy fields use the existing encrypted configuration and scoped permissions;
no database migration or new administrative privilege is added. API/GitOps and
upstream/tool editors share the same inheritance rules. Read-only classification
and retry remain distinct decisions.

The v2.0.1 invocation-security history is unchanged: the legacy global
`VMCP_APPROVAL_URL` hook was removed, and deployments retaining it fail startup.
See `v2.0.1.md`; this work does not restore that hook.

## Database work reduction

- Durable MCP admission inserts the operation and its stream in one dependent SQL
  statement; terminal delivery inserts its event and completes the operation in
  one statement within the existing transaction. Missing/terminated sessions fail
  admission. Event count and encrypted byte bounds share one aggregate scan.
- Resource subscription discovery uses one dispatcher per replica, with up to 32
  active workers and a one-second discovery tick. Active authorization checks,
  ownership fencing, lease renewal and retry cleanup retain their existing behavior.
  No Redis dependency, database migration or durability setting change is required.
- Low-load command counts and limitations are documented in
  `maintainer/READINESS_DATABASE_COST_REVIEW.md`; these changes do not establish
  a new 1,000 TPS capacity acceptance result.

## Pod-local runtime protection (Refs #55)

- Remove PostgreSQL capacity counts and their per-server advisory lock. Defaults
  remain 1,024 total / 128 per virtual server / 16 per principal, now **per pod**.
  Replica count and rolling surge increase the combined allowance; this is not a
  distributed quota. No Redis admission dependency is added.
- Platform operators can edit limits in Settings → Security & audit → Runtime
  protection. Migration 46 adds versioned settings and tenant `admission.read` /
  `admission.update` permissions to the built-in platform-operator template.
  Snapshot refresh distributes settings without per-call settings queries.
- Detached workers retain the local request slot after HTTP disconnect until work
  finishes. Lowering a limit affects new admissions, not running work. Ownership
  fencing, lease expiry, revocation, duplicate-request rejection and strict audit
  are retained. Existing sessions and backups are not removed by this migration.
- Finish a mixed-version rollout before assessing SQL reductions. A rollback
  restores the old DB gate; do not down-migrate while new binaries are running.
