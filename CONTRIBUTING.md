# Contributing to vMCP

Thanks for helping! Bug reports, docs fixes and features are all welcome.

## Ground rules

- Open an issue before large changes so we can agree on the approach.
- **API first**: anything the UI can do must be possible through the REST API. Update
  [`api/openapi.yaml`](api/openapi.yaml) together with the code and keep export/apply in step.
- Secrets are write-only: never return or log upstream headers, client keys, or agent/admin key material.
- Certificate verification is never optional (no "skip verify" switch).
- Core functions must work with PostgreSQL alone. Redis may only back optional features.
- Keep existing comments and docstrings unless your change makes them wrong.

## Setup

Requirements: Go (see `core/go.mod`), Node 20+, Docker (PostgreSQL for tests).

```sh
docker compose -f deploy/docker-compose.yml up -d postgres
export VMCP_TEST_DATABASE_URL='postgres://vmcp:vmcp@localhost:5432/vmcp_test?sslmode=disable'
cd core && go test -race ./...           # database-backed suites skip without the variable
cd ../web && npm ci && npm run lint && npm run build
npx @redocly/cli lint api/openapi.yaml
```

Optional: `VMCP_TEST_REDIS_URL=redis://localhost:6379/0` enables the Redis rate-limit test.

## Pull requests

1. Branch from `main`; keep changes focused.
2. Add tests (integration tests under `core/internal/e2e` and `core/internal/controlplane` show the patterns).
3. Run `gofmt`, `go vet ./...`, the tests, and the web lint/build before pushing.
4. Describe *why* in the PR, and note what you did and did not verify.

By contributing you agree your work is licensed under Apache-2.0.
