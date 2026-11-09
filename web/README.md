# vMCP web UI

React, TypeScript and Vite frontend. See [CONTRIBUTING](../CONTRIBUTING.md) for
prerequisites, backend setup and the complete verification workflow.

## Local setup

```sh
npm ci
npm run dev
```

Dependency setup installs the pinned Prettier version and the tracked repository
pre-commit hook. Repeat `npm ci` in each new checkout. The hook checks the exact
staged web content and rejects formatting violations without rewriting or staging
files. Non-web commits do not require the web dependencies.

## Formatting and verification

```sh
npm run format          # Apply shared formatting after edits
npm run format:check    # Check without modifying files
npm run format:staged   # Check the staged content before a commit
npm run test:format     # Exercise the hook and npm lifecycle gates
npm run lint
npm test
npm run build
npm run test:browser
```

`dev`, `build` and `preview` format automatically before starting. The preview
server serves the last build; run `npm run build` first. CI checks formatting
before building so automatic formatting cannot hide a violation.

VS Code settings and extension recommendations are tracked for both the repository
root and this directory. Other IDEs can use the same `.prettierrc.json` and the
root `.editorconfig`. Use the locally installed formatter, never a global version.
Generated output and the npm-owned lockfile are excluded from formatting.

AI agents must also follow the mandatory formatting and commit instructions in
[AGENTS.md](../AGENTS.md). Do not skip hooks or use `--no-verify`.
