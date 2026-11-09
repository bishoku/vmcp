# Shared web formatting (unreleased)

Release metadata: unreleased developer-tooling change after v2.1.1. No runtime
API change, database migration or release tag is introduced.

- Pin Prettier and share formatting/editor configuration in the repository.
- Normalize handwritten web sources and tests without intentional behavior changes.
- Format automatically before local dev, build and preview commands.
- Install a tracked pre-commit hook during dependency setup. Check staged content,
  including partially staged files; report violations without rewriting or staging.
- Check formatting before builds in the required web CI job, and test the gates.
- Document mandatory local checks for contributors and AI agents across checkouts.

Existing developer checkouts must run `npm ci --prefix web` once to install the
formatter and repository hook. Local hooks can be bypassed outside the supported
workflow; protected-branch CI remains the final enforcement boundary.
