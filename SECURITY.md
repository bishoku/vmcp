# Security policy

This repository is in development. No production-supported release or response
SLA is declared. Only the current main branch receives fixes until a supported
release policy is published. Deployments must evaluate their own risk and the
release gates in `docs/PRODUCTION_DEPLOYMENT.md`.

Report suspected vulnerabilities privately through the repository's GitHub
Security Advisory reporting channel when enabled. If it is unavailable, contact
the repository maintainer privately before sharing exploit details. Do not post
credentials, personal information or sensitive upstream payloads in public
issues. Maintainers should acknowledge reports, agree on disclosure timing and
publish affected/fixed versions in an advisory. There is no promised response
time until maintainers explicitly adopt one.

Dependencies are locked by go.sum/package-lock.json. CI runs vulnerability
checks and image scans; review findings rather than blanket-suppressing them.
Schema, key-envelope and protocol compatibility are separate upgrade concerns.
