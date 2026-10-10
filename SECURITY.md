# Security policy

## Reporting a vulnerability

Please **do not** open a public issue. Use GitHub's private
["Report a vulnerability"](https://github.com/bishoku/vmcp/security/advisories/new) form on this repository.
Include affected version/commit, reproduction steps and impact. We aim to acknowledge reports within a few days.

## Scope

vMCP handles upstream credentials, agent keys and potentially sensitive tool output. Issues of particular interest:
authentication or authorization bypass, secret disclosure (headers, keys, tokens), SSRF through upstream URLs,
TLS verification bypass, PII masking bypass, and cross-session data leakage.

Hardening guidance for operators is in [docs/Security.md](docs/Security.md).
