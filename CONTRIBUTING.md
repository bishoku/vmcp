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

See [the reproducible verification workflow](maintainer/Development.md#reproducible-verification).
Install web dependencies/browser, start disposable PostgreSQL and Redis, set both
integration URLs and run `make verify`. It requires all database/Redis suites,
runs the pinned MCP SDK fixture, and rejects skipped tests. Individual `go test`
runs may skip integration tests without those URLs; they are not release evidence.

`cd web && npm run test:browser` builds the app and serves that fresh output with
Vite preview on port 4174. API mocks disable browser HTTP caching, so using the
development server repeatedly reloads its large module graph and can exhaust
test timeouts. Browser tests use four workers locally and two in CI; failed tests
retain a Playwright trace in `web/test-results/`. The live browser suite remains
separate.


## Pull requests

1. Branch from `main`; keep changes focused.
2. Add tests (integration tests under `core/internal/e2e` and `core/internal/controlplane` show the patterns).
3. Run `gofmt`, `go vet ./...`, the tests, and the web format checks/lint/build before pushing.
4. Describe *why* in the PR, and note what you did and did not verify.

By contributing you agree your work is licensed under Apache-2.0.

## Wiki publishing

The GitHub Wiki is generated from `docs/*.md`, including `Home.md`, `_Sidebar.md`
and `_Footer.md`. Maintainer documents live in `maintainer/` and are not published
to the wiki. Edit user documentation here: publishing replaces wiki Markdown pages
and removes pages absent from `docs/`, while preserving other wiki assets.

The `publish-wiki` workflow runs on pushes to `main` that change `docs/` or the
wiki publishing scripts/workflow. To publish documentation
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
**Helm and image validation (disposable cluster)** job restores locked chart
dependencies, runs semantic chart tests, validates Kubernetes resources, builds
and scans the data-plane image, and exercises installation, an MCP tool call,
upgrade and rolling restart in a temporary Kind cluster. It does not modify an
external cluster, push images, create a release or deploy the application publicly.
Go test JSON output is streamed into the job log and retained for seven days
as the `core-test-results` artifact, including failed test runs.

## Versioned container publishing

Pushing a `vMAJOR.MINOR.PATCH` tag runs `publish-docker-hub`, verifies that commit
through reusable CI, and publishes the Helm runtime images under `barishoku`.
See [Docker Hub setup and Helm image overlays](maintainer/DOCKER_HUB_PUBLISHING.md) for
the required repository secret, image list and versioning procedure.

## Web formatting

Run `npm ci --prefix web` in each checkout. The lockfile pins Prettier and npm's
`prepare` step installs the repository-managed `.githooks/pre-commit` hook. This
sets the checkout's `core.hooksPath` to `.githooks`; reinstalling dependencies
restores it. Docker contexts and source archives without `.git` skip hook setup.
Do not skip npm lifecycle scripts for a development checkout.

Use `npm run format --prefix web` before local development/builds and after the
last edit. `npm run dev`, `npm run build` and `npm run preview` inside `web/` run
formatting automatically first. `npm run format:check --prefix web` is read-only.
Before committing, stage the intended formatted changes and run
`npm run format:staged --prefix web`; the Git hook runs this same check against
staged blobs, including partially staged files. It never formats or stages files
on your behalf. Missing dependencies or malformed/unformatted staged web files
block the commit; non-web commits do not require web dependencies.

Tracked VS Code settings work from the repository root or `web/`. Install the
recommended Prettier extension and use the project-local formatter/configuration;
other IDEs can use the same `web/.prettierrc.json` and `.editorconfig`. The shared
style is two spaces, single quotes, no semicolons, LF line endings and a 100-column
print-width target. Tracked Git attributes preserve LF across Windows/macOS/Linux
checkouts without converting binary assets. Build/test artifacts and the npm-owned
lockfile are ignored.

The required **web** CI job checks the entire web tree before a build can format
anything, then tests the formatting gates themselves. A developer can disable a
local hook, so protected-branch CI is the enforcement boundary; do not bypass
hooks or protected-branch requirements. Formatting does not replace lint/tests.
