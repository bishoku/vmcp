# Import playground data into an existing stack

Run from the repository root with Python 3.9+. No Helm, kubectl or third-party
Python packages are required by the importer. It does not install or upgrade
any stack components.

## Prerequisites

- A running vMCP stack with the mock REST management API (v1.2.0+).
- A running WireMock instance configured through `VMCP_WIREMOCK_URL` on the
  control plane. Its URL must also be reachable from the data plane.
- The existing stack's admin token and access to its control-plane API.
  For Kubernetes, you can expose that API with
  `kubectl -n YOUR_NAMESPACE port-forward svc/YOUR_CONTROL_PLANE_SERVICE 8081:8081`.

## Import

```bash
export VMCP_API_URL='http://127.0.0.1:8081'
# Read the existing admin token without putting it in shell history (Bash).
read -r -s -p 'Admin token: ' VMCP_ADMIN_TOKEN; echo
export VMCP_ADMIN_TOKEN

python3 deploy/helm/vmcp/quickstart/import_playground.py
```

The importer reads the configured WireMock URL from `/api/v1/mock-provider`.
It adds missing Meridian Bank fixtures: 30 teams, 40 projects, 20 mock upstreams
(68 tools), 40 virtual MCP servers, 10 runtime keys and cross-team grants.
Two grants intentionally remain pending/rejected, matching quickstart.
Control-plane reconciliation publishes the saved mock definitions to WireMock
every five seconds; no separate mapping import or global WireMock reset is needed.

Existing slugs, edits, runtime keys and grant decisions are preserved. Rerun the
same command with the same state file after an interruption. Run only one importer
at a time for a given stack. This is create-only: there is no automatic rollback
or update of existing records.

## Options and saved keys

```bash
python3 deploy/helm/vmcp/quickstart/import_playground.py \
  --api-url https://control-plane.example.com \
  --state-file /secure/path/dev-playground.json \
  --issuer https://sso.example.com/realms/vmcp-demo \
  --omni
```

- `--api-url` overrides `VMCP_API_URL`; use the control-plane base URL, without
  `/api/v1`.
- `--state-file` defaults to `.playground-import-state.json` in the current
  directory. It stores one-time runtime keys under `keys` and creation versions
  under `ledger`, using owner-only permissions. Keep it private and backed up;
  custom filenames must also be excluded from version control. Use a separate
  file per stack; a different API URL is rejected for an existing state file.
- `--issuer` sets the issuer for newly created teams. The default is
  `http://localhost:8803/realms/vmcp-demo`. Keycloak, users and SSO identity
  bindings are not provisioned. Runtime keys can be used independently of SSO.
- `--omni` enables Omni on newly created virtual servers and requests reindexing.
  Omni must already be configured in the stack. It does not republish existing
  virtual servers; publish those through the console.

Keys already present in the stack are not recreated or recovered into this file.
If a one-time key was lost, manage it explicitly through the console.
Use `--help` to see all arguments.

## Without Keycloak

Keycloak and OAuth2 login are not required to view or manage the imported data.
Sign in to the console using the existing admin token (for example,
`demo-admin-token`, if that is the token configured in your stack). The bootstrap
admin token has full management access to all teams, projects, upstreams and
virtual servers, regardless of team issuer/group fields. The importer uses the
same token through `VMCP_ADMIN_TOKEN`.

Calling virtual MCP tools requires a runtime credential: use the generated
runtime keys saved under `keys` in the state file. The management admin token
is not a runtime key.
