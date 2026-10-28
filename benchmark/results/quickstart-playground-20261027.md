# Quickstart playground and Keycloak acceptance — 2026-10-27

The working-tree quickstart was tested against the **published v1.2.2 chart and
images** on MicroK8s v1.35.6. No new vMCP binary or application migration is needed.
The isolated namespace was `vmcp-playground-check`, release `playground-check`;
the existing `vmcp-demo` installation was not modified. Test infrastructure used
`--ephemeral` and `--no-port-forward`; temporary loopback forwards were used only
by the acceptance clients.

## Installation and runtime

1. Installed `--mock --omni` without model configuration.
2. Added `--keycloak` to the same installation, keeping the original flags.
3. Verified 3 upstreams, 5 virtual servers, 3 teams, 3 demo projects (4 including
   Default), 2 runtime keys, 4 cross-team approvals and 3 group identity bindings.
4. Initialized real MCP sessions and called every selected tool: customer 2,
   customer-assistant 3, operations 3, order-tracking 2, support 2 — **12 calls
   succeeded** with synthetic WireMock responses.
5. Called Omni `discover_tools` with the chat agent key. Results were nonempty and
   `search_mode=keyword`; no embedding or reranker service was configured.
6. Obtained a real Keycloak token for `demo-crm-owner` from `vmcp-demo-agent`,
   checked the `demo-crm` group and `vmcp-runtime` audience, and verified MCP
   tools/list through the matching identity binding.
7. Completed browser authorization-code/PKCE SSO on localhost for all five users:

| User | Console role | Observed scope |
|---|---|---|
| demo-admin | admin | All 4 projects / 3 upstreams |
| demo-crm-owner | team | 1 owned project / 1 upstream |
| demo-commerce-owner | team | 1 owned project / 1 upstream |
| demo-support-owner | team | 1 owned project / 1 upstream |
| demo-viewer | viewer | Login succeeded with the deliberately changed password; the deliberately added Support membership remained effective |

The realm stores state in database `keycloak`, owned by `vmcp_keycloak`, in the
same PostgreSQL instance as vMCP. Credentials are Secret references. Browser
issuer is localhost:8803, while pod discovery and token/JWKS endpoints use the
internal Service via Keycloak dynamic backchannel routing. Both local and ingress
configuration branches have regression checks; an actual public ingress/TLS login
was not exercised in this run.

## Preservation on rerun

Before repeating the full quickstart, the acceptance client deliberately changed:

- A team name and project name.
- An upstream description and a virtual server description/publication setting.
- Runtime-key description and identity-binding enabled state.
- A cross-team order tool grant to **revoked**.
- Keycloak demo-viewer password and group membership.
- The existing console client's allowed callback URI set.

After rerun, all configuration and versions were preserved (operational upstream
health/discovery timestamps were excluded from equality comparison). All retained
credential and seed-ledger SHA-256 hashes were unchanged. The changed Keycloak
password still authenticated, group memberships and callback URI set were
preserved, and the revoked tool remained absent from runtime tools/list.

The creation ledger records actual API-returned/read creation versions, including
virtual-server creation version 2; it does not assume every resource starts at 1.
It allows interrupted initial approvals to finish only for unchanged recorded
resources. Existing revoked/rejected grants are never approved again. Existing
records without ledger ownership are not treated as new fixtures.

## Automated checks and limits

- **22 Python quickstart/playground regressions passed**, including historical
  release inventories, actual v1.2.2 inventory, optional Omni/Keycloak configuration,
  partial seed, pagination, credential preservation, edited sources, revoked
  grants, and existing Keycloak records.
- Existing Helm chart regression command passed.
- 8 release-automation Python checks passed.
- Python compilation, shell syntax and `git diff --check` passed.
- CI now discovers both quickstart and playground Python test modules.

This is functional acceptance, not a capacity or production security assessment.
The infrastructure is an isolated trial (Keycloak start-dev, HTTP internal links,
explicit demo password grant, synthetic read-only APIs). The test releases and
retained test credentials were removed after verification; the empty test
namespace was retained. No release was published for these script changes.
