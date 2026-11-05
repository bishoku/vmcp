# Issue 31 server acceptance

Run the MicroK8s smoke and full profiles described in [README.md](README.md)
first. Do not run this diagnostic or other suites concurrently with a capacity
measurement on the same host.

```sh
python3 benchmark/kubernetes/authorization_acceptance.py \
  --http-profile --output artifacts/benchmark-runs/authorization-acceptance-$(date +%Y%m%d-%H%M%S)
```

Requires Docker, Go and access to the pinned project dependencies. The runner
builds the current control plane, starts a fresh PostgreSQL container and a real
Keycloak 26.8 container, then two native control-plane processes with the same
database and master key. Every listener binds to loopback; credentials are
synthetic. Existing namespaces, demo containers and IdP accounts are untouched.
The processes and containers are removed in `finally`; SIGKILL cannot clean up.

The real authorization-code/PKCE callback creates a server-held session. Checks
cover administrator identity, custom-only claim mapping login, cross-replica
session use, CSRF denial, live grant revocation and cross-replica logout. Recovery
restarts both replicas after bootstrap replacement, verifies rejection of the
old credential, demonstrates separate named-token revocation, stops the IdP,
starts token-only mode, repairs access, restores IdP login and retires the
temporary recovery credential. This verifies application behavior across two
processes. It does not verify Kubernetes Secret distribution, production IdP
configuration, external secret-store access, or zero-downtime rolling recovery.

`--http-profile` adds six separate, bounded, closed-loop HTTP read profiles with
eight concurrent clients for ten seconds each. It repeats with 10 and 1,000
unrelated assignments. Each stage reports actual total assignments, successful
requests/s and p50/p95/p99 service latency. A fresh connection is opened per
request. The native CP has GOMAXPROCS=2, without a container CPU/memory limit.
There are two teams, 20 upstreams, and ten upstreams visible to the reader.
Control payload capture is enabled, read capture disabled. Historical audit
volume is small and grows with access logging. These are diagnostics, not
open-loop saturation, a long soak, or a payload-capture throughput benchmark.
The declared correctness gate is zero HTTP, transport or response-validation
errors; latency is descriptive and has no invented acceptance threshold.

Reports include source revision, binary SHA-256, image IDs and per-check results.
Raw logs stay under the ignored artifact directory. Review a sanitized copy of
`report.json` before committing evidence. Never export credentials or cookies.

The lower-level policy benchmark isolates evaluation/allocation cost with
10/100/1,000/10,000 assignments and a 50-event audit page:

```sh
cd core
go test -run '^$' -bench BenchmarkPolicyScaling -benchmem ./internal/authz
```

It excludes SQL, HTTP, OIDC, and decryption, and must not be presented as endpoint
capacity. Run it separately from the HTTP and MCP measurements.
