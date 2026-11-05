# Issue 31 merge acceptance — 2026-11-05

Issue: [#31](https://github.com/bishoku/vmcp/issues/31), “v2.0.0: enforce granular control-plane authorization and scoped audit access”.
Branch: `feat/31-v2-control-plane-authorization`, base `5d49db89e89de698bdfdeb4e0c8d04fdb918fc8b`.
The measurements include follow-up changes later committed as `accf72a`. Full source-tree/binary hashes and reproduction manifests are retained locally; public summaries are not a replacement for that evidence.

**Merge is not approved by this report.** The selected capacity run, full backend verification and final frontend checks passed locally. PR CI and reviewer approval remain separate gates. No merge, release, tag, issue closure or CI success is implied.

## Design assessment and follow-up

The proposed design uses explicit permissions and deny-by-default decisions, rather than treating a coarse UI role as authorization. Configuration writes check the policy revision within their transaction; reads use a repeatable snapshot. Verified OIDC claims stay in server-held sessions. Historical audit ownership is pinned to resource epochs, with separate metadata, identity, details and request/response payload permissions. These are appropriate boundaries; substituting current ownership for recorded ownership, or granting payload access from metadata access, would break them.

The first capacity runs exposed a concrete hot path: each strict audit batch sent separate SQL inserts for every event, context and ownership domain. The follow-up groups those inserts by table in the same transaction, retaining `synchronous_commit=on`. Only event IDs newly inserted in that transaction receive dependent context/payload rows. Replayed events cannot overwrite payloads or backfill ownership onto legacy evidence. Tests cover duplicate IDs within a batch, invalid ownership, dependent-row failures, rollback, replay and payload association.

The tenant-isolation test also required repair: it changed deployment identity after seeding immutable resource epochs. The fixture now establishes the tenant before creating resources; the production tenant guard was not relaxed.

The frontend follow-up preserves expiry-driven draft/configuration clearing and corrects stale recovery help text. Delayed capability loading now transfers keyboard focus to the eventual heading without overriding subsequent user focus. Omni runtime credential rejection has a distinct `runtime_access_denied` code; only that code on the preview endpoint avoids resetting independent console authorization. Console denials and the same code on other endpoints still clear configuration. OpenAPI, backend and client regression coverage accompany the distinction.

The comparison generator previously printed hard-coded historical failures and PostgreSQL limits. It now derives findings and resource descriptions from the actual report, with regression coverage.

## Selected capacity contract

The user explicitly selected the documented 1,000 calls/s acceptance profile:

- 300 servers × 200 tools = 60,000 tools; 600 sessions, 1,024 workers.
- One active data-plane replica, 8 CPU, `GOMAXPROCS=8`, `GOMEMLIMIT=4GiB`, 6 GiB pod limit.
- Strict metadata audit, ownership enabled; MCP `tools/call` traffic backed by REST fixture upstreams.
- Tuned PostgreSQL with no CPU/memory limit; application pool maximum 64.
- NodePort routing, 600 seconds at 1,000 calls/s; existing success, latency, integrity, memory and revocation thresholds unchanged.

This does not certify the separate 90,000-tool MCP-upstream full fault/soak profile, a 2,000 calls/s peak, or production ingress/IdP infrastructure.

| Run | Result |
|---|---|
| [Smoke — local evidence](../benchmark/results/README.md) | Passed; 589 successful calls with admission/outcome integrity; revocation rejected on both nodes. |
| [Default full profile, interrupted — local evidence](../benchmark/results/README.md) | 600 calls/s steady passed; 2,000 peak, refresh and node-loss phases failed. Stopped after the user selected the documented profile; not a completed soak. |
| [Original NodePort 1,000 profile — local evidence](../benchmark/results/README.md) | Failed: 690.2 successful calls/s, 69.14% success, p95 1,412.8 ms. Four early connection refusals plus concurrency rejections. Functional/integrity/revocation/memory gates passed. |
| [Original Pod IP comparison — local evidence](../benchmark/results/README.md) | Failed: 733.1 successful calls/s, 73.47% success, p95 1,414.4 ms. Removing NodePort did not remove the capacity bottleneck. |
| [Bulk audit NodePort rerun — local evidence](../benchmark/results/README.md) | Passed: 999.6 successful calls/s, 99.973% success, p95 168.8 ms, p99 366.1 ms; 164 steady errors and 8 dropped demand. |

The accepted bulk run matched all 600,078 successful calls to admission/outcome records, with zero duplicate executions or incomplete operations. Loaded key revocation took 3.761 seconds; no data-plane OOM/restart occurred; peak RSS was 2,184,904,704 bytes (2.03 GiB). One warmup NodePort connection refusal caused 500 warmup errors; warmup is not scored. It remains visible in the raw evidence and is not described as error-free operation.

Raw evidence is retained under `artifacts/benchmark-runs/`; the local run manifests identify checksummed raw archives under `artifacts/benchmark-archives/`. Initial PostgreSQL samples showed WAL write/sync waits, no deadlocks or blocked transactions, no pod OOM, and roughly two CPU cores consumed despite an eight-core limit. These observations motivated the batching change; they do not establish production storage performance.

## Authorization and regression evidence

[Correctness report — local evidence](../benchmark/results/README.md): 66 checks passed against real local Keycloak authorization-code/PKCE login, two native control-plane processes, and fresh PostgreSQL. This includes custom-only mapping login, scoped read/admin denial, CSRF, live grant revocation, shared-session logout, bootstrap rotation, named-token revocation, IdP outage/token-only repair, restored IdP login and retirement of the temporary recovery credential.

The optional HTTP diagnostic separately exercises scoped configuration list/detail, denied cross-team detail, audit metadata/details, request payload and response payload at 12 and 1,002 total assignments. All responses were correct. The first run overlapped other regression diagnostics, so its latencies must not be used as isolated performance evidence. [The isolated rerun — local evidence](../benchmark/results/README.md) also passed all 66 checks. With 12 total assignments the six paths achieved 26–30 correct requests/s at p95 381–416 ms. At 1,002 assignments the audit list achieved 14.9 requests/s at p95 726 ms; other paths remained 25–28 requests/s at p95 392–422 ms. These are closed-loop, fresh-connection service measurements, not a 1,000 requests/s control-plane claim. The diagnostic has a small audit history and no declared capacity/latency SLA.

[Regression summary — local evidence](../benchmark/results/README.md): 477 final Go test/subtest results across nine issue-relevant packages passed with race detection and no skips, using PostgreSQL, Redis, WireMock and the pinned SDK fixture. The initial run had one tenant-fixture failure; the final audit/e2e rerun passed all 74 tests. A subsequent full-repository Go run passed 870 tests across 48 packages with race detection, no test skips and `go vet ./...`, including disposable Kafka/RabbitMQ dependencies. Also passed: 323 web unit tests, web lint/build, API contract validation, 44 Helm/quickstart Python tests and 16 benchmark harness tests.

The final complete Playwright run passed **200/200** scenarios in 1.8 minutes. Presentation fixtures now implement explicit capability responses, detail reads and narrow PATCH requests. Security scenarios keep their own resource/field decisions; a coarse admin label still cannot bypass unavailable decisions. Expired sessions clear drafts, including on identity replacement; failed mutations are not replayed. Historical failed diagnostic logs are retained in the checksummed regression archive, rather than presented as final failures.

The [isolated batch benchmark — local evidence](../benchmark/results/README.md) uses 64 provenance-bearing events: median pipeline 30.61 ms/batch versus bulk 19.27 ms/batch (37% lower), but roughly 481 KB versus 978 KB allocated per batch. The accepted load run remained below its memory thresholds. The [policy benchmark — local evidence](../benchmark/results/README.md) separately exposes repeated whole-policy validation cost; it excludes SQL/HTTP and is not endpoint capacity.

## Remaining merge gates

- PR CI on the final commit and human review; local success is not a GitHub CI result.
- Operator-specific Secret distribution, production IdP recovery and rolling Kubernetes recovery remain deployment-specific evidence; the local two-process drill does not certify them.

See [reproduction instructions](../benchmark/kubernetes/AUTHORIZATION_ACCEPTANCE.md). Full evidence and manifests remain local; public summaries retain aggregate metrics and limitations only.
