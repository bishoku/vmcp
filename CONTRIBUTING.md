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

Requirements: Go (see `core/go.mod`), Node 22+, Docker (PostgreSQL for tests).

See [the reproducible verification workflow](docs/PRODUCTION_DEPLOYMENT.md).
Install web dependencies/browser, start disposable PostgreSQL and Redis, set both
integration URLs and run `make verify`. It requires all database/Redis suites,
runs the pinned MCP SDK fixture, and rejects skipped tests. Individual `go test`
runs may skip integration tests without those URLs; they are not release evidence.


## Pull requests

1. Branch from `main`; keep changes focused.
2. Add tests (integration tests under `core/internal/e2e` and `core/internal/controlplane` show the patterns).
3. Run `gofmt`, `go vet ./...`, the tests, and the web lint/build before pushing.
4. Describe *why* in the PR, and note what you did and did not verify.

By contributing you agree your work is licensed under Apache-2.0.
