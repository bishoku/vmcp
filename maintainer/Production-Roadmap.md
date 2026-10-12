# Production readiness roadmap

This is a maintainer backlog, not a deployment procedure. Operators should use the
[production deployment guide](../docs/PRODUCTION_DEPLOYMENT.md).


The June MCP feature families above are implemented within their documented limits.
The next milestone is reliable, secure operation under real deployment failures.
These items remain open; their order reflects production risk, not release dates.

- [ ] **Least-privilege audit identities:** make exporter startup and audit inserts work with the supplied restricted roles after tenant identity migration; test allowed and forbidden SQL operations with real runtime logins.
- [ ] **Policy and audit parity:** record catalog denial decisions, bind approval to executed/returned content, clarify prompt-content trust, and align background subscription revocation/fallback behavior.
- [ ] **Sustained audit operations:** separately privileged retention runner, large-operation handling, exporter lag/dead-letter metrics and alerts, authenticated SIEM sinks, and representative purge/WAL measurements.
- [ ] **Repeatable correctness gates:** diagnose intermittent mixed-workload failures, validate fresh update/export/apply API examples, broaden browser GitOps/SSO coverage, and require exact-commit tests/scans before release artifacts.
- [ ] **Deployment evidence:** sustained multi-node load with p95/p99 and database budgets; rolling restart, PostgreSQL promotion, LISTEN loss and Redis outage drills; restore configuration, encrypted secrets, audit receipts and legal holds with measured RPO/RTO.
- [ ] **Tenant and routing assurance:** canonical target-origin checks, streaming-aware proxy deadlines and complete deployed cross-tenant credential/session/audit tests. Dedicated databases/processes remain the supported model; shared-schema tenant switching is a separate future design.
- [ ] **Management and supply-chain polish:** explicit per-object description-drift acknowledgment, complete service image scanning, artifact provenance/SBOM/signing and documented support/security response commitments.

Production acceptance also needs organization-owned secret custody, access reviews,
retention/privacy decisions and an independently administered immutable audit sink
where required. Code changes alone do not establish SOC 2, ISO 27001 or GDPR compliance.
See [production deployment](../docs/PRODUCTION_DEPLOYMENT.md),
[audit operations](../docs/AUDIT_OPERATIONS.md) and [security operations](../docs/SECURITY_OPERATIONS.md).
