# Prioritized readiness hardening — issue 55

Scope: the priority fixes from the readiness validation, excluding SAML/SCIM.

## Implemented boundaries

- Circuit breaker completion owns a generation; ignore releases a half-open
  probe, stale completions cannot mutate its successor.
- Virtual/Omni MCP arguments use bounded offline compiled schemas before
  approval consumption and after masking. REST retains its existing validation
  and error contract. Invalid schemas remain visible but cannot execute.
- Explicit operator retry/read-only settings use existing encrypted invocation
  configuration, revisions, audit mutations, upstream-scoped permissions and
  GitOps export/apply. No runtime client can supply these settings. Tool overrides
  can change these execution preferences, never weaken inherited identity rules.
- Existing form controls, modal layout, draft guards and save review are reused.
  The effective display distinguishes an explicit retry preference from the
  approval/session-verification prohibition that prevents replay.
- Web header protection, model egress/private CA and explicit legacy opt-in have
  documented rollout requirements. See the unreleased notes before upgrading.
- Critical snapshot, discovery, health, audit, delivery and stream workers have
  scoped panic guards. A supervisor marks the HTTP gate unready and signals
  controlled shutdown. It logs function names, not panic values or stack arguments.
  Unsupervised fatal/runtime/OOM failures still use process restart; this is not
  universal recovery or a promise to complete an ambiguous side effect.
- Logging, exporter backlog, runtime/GC, delivery renewal, admission lock and probe
  metrics supply evidence before invasive performance optimization.

## Intentionally preserved guarantees

Authoritative session/JWT checks, 1-second renewal/explicit cancellation polling,
15-second ownership fencing, durable audit admission and existing payload limits
are unchanged. No correctness guarantee is traded for a speculative QPS gain.
KMS providers, shared-schema tenancy, ANN and generic ABAC are not priority fixes
without a demonstrated customer requirement or performance bottleneck.

## Acceptance and remaining operational gates

Use local PostgreSQL race tests and CI for API/persistence/GitOps, revocation,
legacy rotation, schema/annotation/retry boundaries, guarded worker failure and
exporter metrics. Browser tests cover mobile/desktop, inheritance, staging, save,
removal, read-only access and existing identity/approval composition. Real nginx
response tests must cover HTML, runtime config, SPA paths and errors.

Production throughput, PostgreSQL promotion/PITR, independent WORM retention and
recovery RPO/RTO require a representative authorized test environment. A local
fixture/CI run does not establish these. Run the existing acceptance harness with
current source SHA, a single hot server and distributed traffic, short and long
MCP/REST operations, payload capture/PII, database delay and node loss. Record
errors, p95/p99, pool/lock waits, renewal failures, audit reconciliation and achieved
RPO/RTO. Do not advertise the historical 1,000 calls/s result as current capacity.

No production data migration, live secret rotation, infrastructure failover,
merge, version bump or release is performed by the code change itself.

## Local verification (2026-11-08)

- `go test -race -count=1 ./...` passed with disposable PostgreSQL 17.
  Redis, broker and WireMock dependent integrations require the CI service matrix;
  local success is not a claim that those optional local cases ran.
- Targeted race regressions cover operator retry off/never/once despite upstream
  hints, approval-bound replay prohibition, pre/post-masking schema validation,
  private CA/egress, delegation, GitOps and legacy rotation.
- Web: 327 unit tests passed; lint, build and OpenAPI validation passed
  (107 operations, six examples). The existing large-bundle warning remains.
- The invocation security browser test passed at mobile and desktop sizes;
  screenshots were inspected for form layout and overflow.
- Real nginx 1.27 responses for HTML, SPA fallback, runtime configuration,
  health and a proxy 502 include all four security headers; runtime configuration
  also retains `Cache-Control: no-store`. Chromium loaded the login page under
  that CSP without policy violations.

Required GitHub checks must additionally pass on the PR's current head before
merge. The source readiness report is an unrelated, untracked user file and is
not included in this implementation.
