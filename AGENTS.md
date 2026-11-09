# Contribution workflow

- Open a GitHub issue before starting a fix, feature, or maintenance change. Reuse an existing issue when it already covers the requested work.
- Work on an issue-linked branch (for example `feat/37-invocation-security` or `fix/42-session-expiry`), never directly on `main`.
- Open a pull request targeting `main` and reference the issue. Use `Closes #N` only when the PR completes that issue; otherwise use `Refs #N` and leave remaining work visible.
- Run relevant local checks and wait for all required GitHub checks on the current PR head. Fix failures on the branch; do not bypass or weaken branch protection.
- Merge through GitHub only when the user has authorized merging and required checks pass. Never push implementation commits directly to `main`.
- Prepare version metadata and release notes in a PR. Publish an authorized release only from the verified merged commit; do not move existing release tags.
- Exclude generated credentials, certificates, local deployment state, and unrelated user files from commits.

The invocation-security release target tracked by #37 is v2.0.1. Document the legacy approval-hook removal explicitly even though this target is a patch version.

# Web formatting (all contributors and AI agents)

- Run `npm ci --prefix web` in each fresh checkout. It installs the locked formatter
  and the tracked `.githooks/pre-commit` hook; do not bypass it or use a global formatter.
- Before starting the web app or building it locally, run `npm run format --prefix web`.
  The supported `npm run dev`, `npm run build` and `npm run preview` commands also
  format automatically through their npm lifecycle hooks. Do not call Vite or the
  compiler directly to bypass these checks.
- After the final web edit and before staging/committing, run
  `npm run format --prefix web` and `npm run format:check --prefix web`.
  After staging, run `npm run format:staged --prefix web`. Never commit web code
  unless both checks pass; re-stage only the intended changes after formatting.
- Do not disable hooks, use `--no-verify`, suppress formatting violations, or weaken
  formatter ignores/CI to get a commit through. Fix the files and rerun the checks.
- Preserve the pinned formatter, shared configuration, editor settings, hook setup
  and CI gate in the repository so every developer checkout uses the same rules.
- Formatting does not replace lint, build or relevant tests. Keep formatting-only
  changes separate from behavior changes, and exclude generated artifacts and local state.
