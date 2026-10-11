# Authorization and session consistency (P03)

## Authorization freshness

The data plane confirms the PostgreSQL configuration revision through `LISTEN/NOTIFY` and fallback polling (normally every five seconds). `VMCP_AUTH_MAX_STALE` is a positive Go duration, default `30s`. Polling is capped at one third of that duration when necessary. The confirmation timestamp is taken **before** the database work, so a slow query cannot extend the freshness window after it completes.

An unchanged, successfully queried revision renews confirmation. A newer revision renews confirmation only after the complete snapshot, including decrypted credentials, loads successfully. Query failure, decryption failure and revision rollback preserve the last good snapshot for inspection, but cannot extend its authorization validity. Once confirmation is at least the configured age, MCP POST, GET and DELETE return HTTP 503, including anonymous virtual servers. A successful refresh restores service automatically. The bound concerns new authorization admission; previously dispatched calls follow the in-flight policy below.

`GET /readyz` returns JSON containing `ready`, `revision`, `observedRevision`, `confirmedAt`, `ageNs`, `maxAgeNs` and a generic `reason`, with HTTP 503 when stale. Refresh failure inside the grace window appears as a degraded reason while readiness remains true. Credentials, snapshot contents and raw database/decryption errors are excluded. `/healthz` remains an independent liveness check. This is an authorization freshness probe, not a direct guarantee of session-store or upstream health.

A revocation normally propagates via notifications or polling. Under notification loss, database outage or a failed credential load, a node cannot authorize indefinitely from an old snapshot: it refuses new admission at the freshness deadline. Use the same bound on all nodes. Very short values increase database work and can reduce availability during ordinary query latency.

## Session ownership and authoritative termination

Every shared-session request queries PostgreSQL, even when the node already holds the session and upstream connections. There is **no positive cache lease** for session validity. Pool acquisition and store operations have a five-second timeout, further bounded by the caller's deadline. Database read failures return 503; the local cache never supplies authorization fallback. Use the writable primary or an equivalently consistent endpoint, not an asynchronously replicated read endpoint.

Ownership is persisted as authentication mode, principal, immutable key/binding ID, and, for OAuth, verified issuer and subject. Two subjects sharing a binding cannot use each other's sessions. A new token for the same identity can resume the session. Changing authentication mode or deleting/recreating a key or binding with the same slug requires a new session. Credentials, expiry, grants and IP restrictions are checked again on each request.

DELETE first writes an authoritative `terminated_at` tombstone. Write failure returns 503 and retains the local session. After successful DELETE, the next authoritative session lookup on any node returns unknown session (404); no notification or idle reap is required. Requests whose lookup preceded termination may already be admitted. Saving an existing ID cannot revive a tombstone, and initialization/touch updates exclude terminated rows. The periodic session reaper purges tombstones after the 30-minute session idle TTL; absence also rejects the ID. Other nodes retire their cached upstream clients on their next lookup or idle reap.

## Configuration generations and in-flight work

Upstream clients are keyed by a SHA-256 fingerprint of connection settings, including URL, ordered fallbacks, transport, decrypted headers, TLS and OAuth configuration. Fingerprints cover secrets and must not be logged. Snapshot changes retire superseded clients, including idle clients; lazy acquisition independently compares the generation. OAuth source registry reconciliation removes obsolete credential entries, and already-admitted old requests cannot reinsert them into that registry.

Retirement stops reuse immediately. Leased clients remain alive until their admitted calls release them; idle retired clients close asynchronously with a five-second close budget. Requests use a 60-second tool-call deadline. Retired clients and token sources are released through normal Go object lifetime; this is not a secure memory-zeroization guarantee.

Authorization is rechecked after body reading, after session resolution, and before each tool dispatch, including retries and upstream-session recovery. A configuration revision change while waiting for an upstream initialization or retry returns HTTP 409 (`configuration changed; retry`) before a further tool dispatch. Stale confirmation returns 503; expired/revoked credentials return the normal 401/403. This conservative behavior avoids continuing with a superseded routing or masking policy. A caller must decide whether to retry a failed operation based on its tool's semantics.

A call dispatched before a revocation, credential rotation or DELETE may finish within its existing deadline and return its result. Automatic retries must pass authorization again. There is no atomic transaction between the authorization check and an external tool's side effect, and cancellation cannot undo an upstream side effect. SSE reconnects and active streams use the same freshness/ownership contract and reauthenticate periodically; see [delivery](MCP_DELIVERY.md).

## Upgrade and rollback

Migration `00017_session_ownership.sql` adds ownership and termination columns. Existing sessions receive `owner_mode='legacy'`: their original OAuth subject cannot be recovered safely, so current nodes reject them and clients must initialize new sessions. Upgrade tests preserve the old lifecycle fields; rollback drops the new fields and does not preserve tombstones or identity ownership enforcement.

Drain all old data-plane binaries, apply migrations through the control plane, then replace all data-plane nodes before restoring traffic. Mixed versions cannot enforce this contract because old binaries ignore ownership and tombstones. Treat rollback as a coordinated maintenance operation: drain traffic, discard existing session rows, and reconnect clients before restoring traffic. Back up configuration and secrets before schema changes.

## Regression coverage

- `core/internal/snapshot/freshness_test.go`: unchanged confirmations, database failure, decryption/load failure, observed revision rollback, exact expiry, slow-query timing and recovery.
- `core/internal/snapshot/syncer_test.go`: real PostgreSQL ciphertext corruption expires authorization and repair restores it.
- `core/internal/virtual/consistency_test.go`: warm-cache store failures and termination, visible DELETE failure, all-method stale rejection, generation coverage, leased-client retirement and configuration change during upstream initialization without tool dispatch.
- `core/internal/e2e/consistency_test.go`: independent nodes with warm caches, key revocation, grant changes, endpoint/header rotation, immediate cross-node termination, persisted OAuth ownership, subject/issuer isolation and replacement binding identity.
- `core/internal/store/migrations_test.go`: upgrade and rollback from schemas 15 and 16, with explicit legacy ownership defaults.
- `core/internal/oauthtoken/oauthtoken_test.go`: current sources remain shared; obsolete sources are removed and cannot repopulate the registry.

The gateway also implements durable audit admission, bidirectional SSE and negotiated
client features. Deployment-specific HA/restore/load verification and the remaining
[production roadmap](../README.md#roadmap-to-production-readiness) are still required.
