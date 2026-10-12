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

## Wiki publishing

The GitHub Wiki is generated from `docs/*.md`, including `Home.md`, `_Sidebar.md`
and `_Footer.md`. Edit documentation here: publishing replaces wiki Markdown pages
and removes pages absent from `docs/`, while preserving other wiki assets.

The `publish-wiki` workflow runs on every push to `main`. To publish documentation
without a release, select **Actions → publish-wiki → Run workflow** on `main`.
It creates commits only in the wiki repository; it creates no release or tag.
Unchanged content produces no wiki commit.

The wiki must already contain an initial page. The workflow tries `GITHUB_TOKEN`
by default. If GitHub denies access to the separate wiki repository, add a repository
Actions secret named `WIKI_TOKEN`: use a personal access token belonging to an account
with write access to the wiki (a classic token with `public_repo` scope for this
public repository). Never commit the token. Then rerun the failed workflow.

To preview generated Markdown locally:

```sh
python3 -m unittest discover -s scripts -p 'test_prepare_wiki.py'
scripts/prepare-wiki.sh /tmp/vmcp-wiki-preview
```

Links between documentation pages become wiki links; links to repository files
point to `main` on GitHub. Fenced code examples remain unchanged.

## CI checks

The `ci` workflow runs Go race/integration tests and frontend checks. Its
**Deployment validation (no deploy)** job lints and renders the Helm chart,
builds and scans the data-plane image. These are validation checks on the runner: no cluster is
modified, image pushed, release created, or application deployed.
Go test JSON output is streamed into the job log and retained for seven days
as the `core-test-results` artifact, including failed test runs.
