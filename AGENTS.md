# Contribution workflow

- Open a GitHub issue before starting a fix, feature, or maintenance change. Reuse an existing issue when it already covers the requested work.
- Work on an issue-linked branch (for example `feat/37-invocation-security` or `fix/42-session-expiry`), never directly on `main`.
- Open a pull request targeting `main` and reference the issue. Use `Closes #N` only when the PR completes that issue; otherwise use `Refs #N` and leave remaining work visible.
- Run relevant local checks and wait for all required GitHub checks on the current PR head. Fix failures on the branch; do not bypass or weaken branch protection.
- Merge through GitHub only when the user has authorized merging and required checks pass. Never push implementation commits directly to `main`.
- Prepare version metadata and release notes in a PR. Publish an authorized release only from the verified merged commit; do not move existing release tags.
- Exclude generated credentials, certificates, local deployment state, and unrelated user files from commits.

The invocation-security release target tracked by #37 is v2.0.1. Document the legacy approval-hook removal explicitly even though this target is a patch version.
