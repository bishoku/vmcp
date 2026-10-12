# Historical network security verification

This records the initial network-security phase, not current feature availability.
For current behavior, see [Network security](../docs/MCP_NETWORK_SECURITY.md).

## Regression coverage

- `core/internal/netpolicy/egress_test.go`: default address denials, explicit private/loopback grants, mapped/transition forms, mixed DNS answers, rebinding with direct IP dialing, exact host restrictions, invalid configuration, credential-bearing URLs, redirects/proxies, TLS trust and hostname verification.
- `core/internal/virtual/ingress_test.go`: origins across POST/GET/DELETE, native clients, duplicate/version headers, media negotiation, body limit, GET authentication and session ownership across methods.
- `core/internal/upstream/sse_test.go`: same-origin legacy interoperability, cross-origin endpoint credential-exfiltration prevention, denied loopback over both transports.
- `core/internal/oauthtoken/oauthtoken_test.go`: denied token destination receives no client credentials.
- Existing PostgreSQL/Redis integration, OAuth, CA/mTLS and discovery tests continue exercising approved paths.

These are implementation regression checks, not a penetration test or full MCP certification. The remaining phases retain their own release gates.

### Verification result

On 10 October 2026, `go test -race -count=1 -json ./...` passed with **253 test/subtest pass events, zero failures and zero skips**, using isolated PostgreSQL 17 and Redis 7. The suite includes `core/internal/controlplane/network_security_test.go`, verifying that connection tests, refresh/discovery, apply preflight and health/fallback probes cannot contact a denied destination. `go vet ./...`, `git diff --check` and `docker compose -f deploy/docker-compose.yml config --quiet` passed. The temporary full test log is `/tmp/vmcp-p02-tests.jsonl`. No frontend code changed; frontend checks were not rerun. Temporary test containers were removed after verification. P03 has not been started.
