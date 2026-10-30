# Try vMCP on Kubernetes

The quickstart installs a small trial stack into your selected Kubernetes cluster:
PostgreSQL, authenticated Redis, migrations, the console, one control plane and two
data planes. Omni MCP, a seeded WireMock playground, Keycloak, Kafka, RabbitMQ,
independent audit exporters and monitoring integration are optional. It creates two Helm releases: `<release>` and `<release>-infra`.

This is a trial installer, not a production configuration. Internal connections
are plaintext, infrastructure has one replica, runtime database roles have broad
DML permissions, private upstream address ranges are allowed, and application
NetworkPolicies are disabled. Use an isolated development cluster or namespace.
For production, follow [Production deployment](PRODUCTION_DEPLOYMENT.md).

## Prerequisites

- A clone of this repository containing `deploy/helm/vmcp/quickstart.sh` and its
  adjacent `quickstart/` directory. The published v0.0.2 source predates this script;
  use the current main branch to try that application release.
- Bash, Python 3.9+, kubectl, Helm 3.17+, and Kubernetes 1.27+.
- Permission to create namespace-scoped workloads, Secrets, PVCs and Helm releases,
  and read nodes, cluster version and StorageClasses. Monitoring also reads CRDs.
- A default StorageClass, or `--storage-class NAME`. `--ephemeral` avoids PVCs but
  loses all dependency data when their pods are replaced.
- Network access to GitHub Releases, Docker Hub and (with Keycloak) quay.io. Allow roughly 2 CPUs/4 GiB
  of spare capacity for the base trial, and additional capacity for brokers.
  Actual scheduling needs depend on the cluster and enabled components.

The installer prints the selected context and passes it explicitly to every
cluster command. It does not switch your kubeconfig context or create a cluster.

## Optional Omni MCP

Use the current repository quickstart with v1.2.0 or later. For example:

```sh
./deploy/helm/vmcp/quickstart.sh \
  --context microk8s --version v1.3.0 --omni \
  --storage-class microk8s-hostpath \
  --web-host vmcp.kube.local --gateway-host mcp.kube.local \
  --ingress-class nginx --no-port-forward --timeout 20m
```

`--omni` installs the release's digest-pinned Omni image, configures internal
control-plane/data-plane connections and creates a bridge credential in the
existing quickstart Secret. Reruns preserve this credential and existing database,
admin and encryption credentials; adding Omni to an existing trial is supported.
The flag defaults to off. It requires no embedding or reranker model: discovery
uses keyword search. Anonymous discovery stays disabled. Publish virtual servers
through Omni in the console and use their normal agent runtime credentials;
publication does not bypass tool approvals or runtime authorization.

With ingress the endpoint is `http://mcp.kube.local/mcp/v1/omni` (HTTPS when
`--tls-secret` or `--external-https` is supplied). Without ingress, an additional port-forward serves
`http://localhost:8802/mcp/v1/omni`; use `--omni-port PORT` to change it. Enabled
local ports must be distinct. `--no-port-forward` prints the commands instead of
starting them. Configure MCP clients with Streamable HTTP and
`Authorization: Bearer <agent-runtime-key>`; the internal bridge secret is not a
client credential. The installer prints the endpoint and access instructions.

Embedding and reranking remain optional, separately operated services. This flag
does not deploy models or add model configuration flags; see
[Omni installation](Omni-MCP-Installation.md) and
[reranking configuration](Omni-MCP-Reranking.md) for Helm model settings. Keep such
settings in a separate managed installation rather than assuming quickstart
reruns preserve manual Helm overrides.

## Ready-to-use playground (`--mock`)

Use the current repository script with v1.2.0 or later. No model endpoint is needed:

```sh
./deploy/helm/vmcp/quickstart.sh \
  --context microk8s --version v1.3.0 --mock --omni \
  --keycloak-host sso.kube.local \
  --storage-class microk8s-hostpath \
  --web-host vmcp.kube.local --gateway-host mcp.kube.local \
  --ingress-class nginx --no-port-forward --timeout 20m
```

The flag starts digest-pinned WireMock 3.13.2 and configures the control plane's
mock REST provider. It creates the following **missing** resources through the
management API, using the bootstrap admin token in memory. The scenario is a
fictional retail, business and corporate bank, **Meridian Bank**, built for
agentic banking use cases; all data is synthetic and English-only.

| Resource | Count | Examples |
|---|---|---|
| Teams | 30 | `retail-digital-banking`, `fraud-strategy`, `financial-crime-compliance`, `lending-origination`, `treasury-markets` |
| Projects | 40 | `retail-mobile-assistant`, `fraud-investigation-workbench`, `aml-alert-triage`, `personal-loan-journey` |
| REST upstreams | 20 (68 tools) | `core-banking-accounts`, `payments-hub`, `card-management`, `fraud-detection`, `aml-screening`, `loan-origination`, `treasury-markets-data` |
| Virtual servers | 40 | `retail-banking-assistant`, `payments-copilot`, `contact-center-agent-assist`, `fraud-triage`, `credit-decisioning`, `corporate-treasury` |
| Runtime keys | 10 | `retail-channels-agent`, `contact-center-agent`, `fraud-ops-agent`, `financial-crime-agent`, `lending-agent`, … (each server belongs to one key) |
| Cross-team grants | all approved except two | `genai-sandbox`: `initiate_domestic_payment` left **pending**, `screen_party` **rejected** |

The upstreams share one consistent data set: 10 customers (`CUST-100101` …
`CUST-100107` individuals, `CUST-200201`/`CUST-200202` SMEs, `CUST-300301` a
corporate), their accounts, transactions with running balances, cards, loans,
fraud alerts, AML cases, KYC records, payments, portfolios, complaints and more.
IDs returned by one tool are accepted by the others (for example
`list_customer_accounts` → `list_account_transactions` → `initiate_domestic_payment`
→ `get_payment_status`). Every tool has typed input parameters with descriptions,
patterns and enums, and a documented response schema; write tools (payments, card
freezes, disputes, applications, notifications) return templated, request-aware
responses. Virtual servers carry agent instructions (`guidance.instructions`)
describing safe banking behaviour, such as confirming payments or tipping-off rules.

Try `list_customer_accounts` with `customerId=CUST-100101` on
`retail-banking-assistant`, or approve the pending `genai-sandbox` grant in the
console. All servers require credentials; anonymous access stays off.
`--omni` publishes newly created demo servers and enables keyword discovery; without
it, they work through their normal virtual MCP endpoints. Adding `--omni` later
does not republish existing records: use the console to publish those you want.

Installs created with an older quickstart keep their previous `demo-*` fixtures:
seeding is create-only, so the banking resources are added next to them. Remove the
old records in the console if you no longer need them.

One-time runtime keys are stored in the retained quickstart Secret; the installer
does not print them. Use [`demo.sh`](#demo-helper-demosh) to read them.
If a key already exists but its one-time secret was lost/rotated in the console,
quickstart does not rotate or recreate it; manage the key explicitly in the console.
WireMock mappings are restored from saved vMCP mock definitions by the existing
control-plane reconciliation; no global WireMock reset is used by this installer.

## Optional Keycloak and group identities (`--keycloak`)

Keycloak 26.4.7 is digest-pinned, runs in trial `start-dev` mode, and uses the
same PostgreSQL **instance** with separate database `keycloak`, LOGIN role
`vmcp_keycloak`, and password. The initialization creates only missing database/
role objects; application tables and credentials are not reused. Keycloak's state
survives restarts in PostgreSQL (subject to the chosen PVC/ephemeral storage).

Add it to the playground in a later run, retaining the original flags:

```sh
./deploy/helm/vmcp/quickstart.sh \
  --context microk8s --version v1.3.0 --mock --omni --keycloak \
  --storage-class microk8s-hostpath \
  --web-host vmcp.kube.local --gateway-host mcp.kube.local \
  --keycloak-host sso.kube.local --ingress-class nginx \
  --no-port-forward --timeout 20m
```

Set `--keycloak-host sso.kube.local` **also on the first `--mock` run** if you plan
to add Keycloak with that hostname later. This records matching team issuers before
SSO is enabled. Existing issuer/group mappings are never silently changed; if you
change hosts later, review the affected demo teams in the console. Without ingress,
omit all host flags: Keycloak uses `http://localhost:8803` (`--keycloak-port` changes
it). The script starts/prints its extra loopback port-forward like the Omni one.

For ingress, all three hostnames must resolve to your ingress address in the
browser. `--tls-secret` also covers the Keycloak ingress; the certificate must
include that hostname. The internal discovery URL uses the Keycloak Service while
JWT issuer and browser login use the public URL. Dynamic backchannel endpoints
keep token exchange/JWKS reachable from pods even in localhost mode. The script
configures the console OIDC client and group-to-role settings, and an RS256 agent
OIDC verifier with audience `vmcp-runtime`.

| Demo user | Group / console access |
|---|---|
| `platform.admin` | `vmcp-platform-admins` → platform admin |
| `security.auditor` | `vmcp-auditors` → global viewer |
| 105 team members, e.g. `oliver.harrison` | their team slug (e.g. `core-banking-platform`) → that team's scope |

Every team has 3–4 members (`first.last`, `@meridianbank.example`) with banking
roles; `./deploy/helm/vmcp/demo.sh users` lists them all. The legacy
`demo-admins`/`demo-viewers` groups remain mapped so earlier installs keep working.

A `groups` claim mapper is configured on the confidential `vmcp-console` client
with PKCE and an exact console callback URL. The realm is `vmcp-demo`. Keycloak
can be installed without `--mock`; the 30 team mappings and SSO users are still
created, but upstreams/projects/servers and runtime fixtures are not.

With both flags, one `<team>-sso` identity binding per team grants its members
runtime access to the team's virtual servers. A separate public client
`vmcp-demo-agent` has a **trial-only password grant** for exercising these
identities: `demo.sh token <user>` returns an access token (it port-forwards to
Keycloak and uses `client_id=vmcp-demo-agent`, `grant_type=password`).
Use the token as a runtime Bearer credential. Its group and audience claims are
verified; console viewer/admin roles do not themselves grant MCP runtime access.
The normal runtime keys remain usable independently of Keycloak.

All demo users share one initial password (`demo.sh password`). The Keycloak
bootstrap admin is `admin` (`demo.sh credentials`). All initial credentials and the
console client secret are retained, not reset. Keycloak API seeding creates only
missing realms, groups, clients and users; it does not reset edited passwords,
client configuration or group memberships. If you change a client secret, issuer,
callback URL or bootstrap admin password, keep the corresponding quickstart Secret/
Helm configuration in sync yourself; the installer will not undo your changes.
Allow additional memory for WireMock and Keycloak (each has a 1 GiB pod limit).

## Demo helper (`demo.sh`)

`deploy/helm/vmcp/demo.sh` answers "who can log in, with what, and where?" without
kubectl/base64 commands. Catalog commands work offline; credential commands read
the `<release>-quickstart` Secret once through your kubectl context. Use the same
`--context`, `--namespace` and `--release` as the quickstart (defaults:
current context, `vmcp-demo`, `vmcp-demo`). Add `--json` for machine-readable output.

| Command | Output |
|---|---|
| `demo.sh users [--team SLUG] [--passwords]` | SSO users with name, team and role |
| `demo.sh user USERNAME` | One user: groups, console role, team servers, initial password |
| `demo.sh password` | Shared initial SSO password only (pipe-friendly) |
| `demo.sh teams` / `demo.sh team SLUG` | Teams with members, projects, servers and owned upstreams |
| `demo.sh projects` | Projects, owner teams and their servers |
| `demo.sh servers [--team SLUG]` / `demo.sh server SLUG` | Endpoint URL, tools, upstreams, grants, agent key, instructions |
| `demo.sh upstreams` | Mock upstreams and tools |
| `demo.sh keys` / `demo.sh key SLUG` | Runtime agent keys (all, or one raw value) |
| `demo.sh credentials` | Admin token, Keycloak admin and user passwords, runtime keys, URLs |
| `demo.sh urls` | Console, gateway and Keycloak URLs (ingress or port-forward) |
| `demo.sh token USERNAME` | Keycloak access token for runtime calls as that user |

```sh
./deploy/helm/vmcp/demo.sh credentials
./deploy/helm/vmcp/demo.sh users --team fraud-strategy
KEY=$(./deploy/helm/vmcp/demo.sh key retail-channels-agent)
```

`demo.sh` prints secrets only for the commands that explicitly request them.

## Preservation on reruns

Keep the complete desired flag set on each run; omitting an optional service flag
removes/disables its Helm resources. The application configuration seed performs
no PUT, PATCH, DELETE or bulk replace. Existing names, tools, publication settings,
ownership, keys, identity bindings and grant decisions remain intact. Deleted
fixtures count as missing and may be recreated; this is not a reset command.

Creation versions are retained in `playground-seed-ledger` inside the quickstart
Secret. A partially completed seed can finish initial cross-team approvals only
while the affected upstream, project and virtual server still match their recorded
creation versions. Edited resources are skipped, and revoked/rejected grants are
never reapproved. Seed failures leave the partial installation for inspection and
retry. Seed state and one-time keys are not written to local values files or logs.

This preservation concerns API/Keycloak data and retained credentials. Rerunning
quickstart still applies its generated Helm values; it does not merge arbitrary
manual Helm overrides. PostgreSQL PVCs and the quickstart Secret remain on normal
uninstall; `--uninstall --purge` explicitly removes them. `--ephemeral` PostgreSQL
loses both vMCP and Keycloak databases on pod replacement.

## Start

From the repository root:

```sh
./deploy/helm/vmcp/quickstart.sh --help
./deploy/helm/vmcp/quickstart.sh --context YOUR_CONTEXT --version v0.0.2
```

It downloads the selected release's chart, checksums and image inventory, checks
asset checksums and image digests, creates/reuses credentials, installs dependency
workloads, creates database roles, runs migrations, grants runtime/export privileges,
creates broker destinations when selected, and installs vMCP. Helm waits for readiness.
The packaged application chart includes its locked dependencies; the quickstart
uses its own small infrastructure chart with pinned official images instead of
turning on the production chart's optional Bitnami subcharts.

By default it keeps two localhost-only port forwards running:

- Console: `http://localhost:3300`
- Gateway: `http://localhost:8801`

The installer does not print the admin token; read it with
`./deploy/helm/vmcp/demo.sh credentials`. Sign in using that token, register your HTTP
upstreams, create a virtual server, and issue an agent key from the console.
Gateway endpoints are `http://localhost:8801/mcp/v1/virtual/<slug>`.

Ctrl-C stops forwarding; the installed stack stays running. Use
`--no-port-forward` to exit after installation and print forwarding commands.
`--web-port` and `--gateway-port` select different local ports and update runtime
URLs. Internal cluster workloads should use the data-plane Service endpoint.

## Optional brokers and exporters

```sh
# Brokers only: Kafka topic and RabbitMQ queue are created, but no exporters run.
./deploy/helm/vmcp/quickstart.sh --context YOUR_CONTEXT --kafka --rabbitmq

# Independent exporters: each flag also enables its corresponding broker.
./deploy/helm/vmcp/quickstart.sh --context YOUR_CONTEXT \
  --exporter kafka --exporter rabbitmq --no-port-forward

# Your own HTTP receiver, reachable from cluster pods.
./deploy/helm/vmcp/quickstart.sh --context YOUR_CONTEXT \
  --exporter http --http-export-url https://receiver.example.com/audit
```

Exporters use distinct consumers (`quickstart-http`, `quickstart-kafka`,
`quickstart-rabbitmq`) and a separate database LOGIN role. Kafka uses the
`vmcp-audit` topic; RabbitMQ uses a durable `vmcp-audit` queue through the default
exchange. Generate audit events by configuring or calling virtual servers, then
inspect broker messages and exporter logs. Kafka uses unauthenticated plaintext
inside the trial cluster; Redis and RabbitMQ use generated passwords but no TLS.
Avoid credentials embedded in command-line URLs; configure a suitable trial receiver.

`--monitoring` enables ServiceMonitors, Prometheus rules and dashboard ConfigMaps.
It requires existing Prometheus Operator CRDs, a Prometheus instance selecting the
resources, and a Grafana dashboard sidecar. It does **not** install Prometheus or
Grafana. Use `--monitoring-release NAME` to match your Prometheus selection label.

## Customize access and storage

```sh
./deploy/helm/vmcp/quickstart.sh --context YOUR_CONTEXT \
  --namespace my-vmcp --release my-vmcp \
  --storage-class standard --data-plane-replicas 3 \
  --web-port 3301 --gateway-port 8802 --timeout 20m
```

For HTTP ingress, first provision DNS and an ingress controller. For example,
with MicroK8s, an existing `nginx` IngressClass, `microk8s-hostpath` storage and
wildcard DNS for `*.kube.local` pointing to your ingress controller:

```sh
./deploy/helm/vmcp/quickstart.sh --context microk8s \
  --storage-class microk8s-hostpath \
  --web-host vmcp.kube.local --gateway-host mcp.kube.local \
  --ingress-class nginx --exporter kafka --exporter rabbitmq \
  --no-port-forward --timeout 20m
```

This installs the base stack plus both brokers and their audit exporters. Open
`http://vmcp.kube.local`; gateway endpoints use
`http://mcp.kube.local/mcp/v1/virtual/<slug>`. Without `--tls-secret`, ingress has
no TLS configuration and nginx HTTPS redirects are disabled. Check your actual
IngressClass and StorageClass names with `kubectl get ingressclass,storageclass`.

For HTTPS ingress, also provision a TLS Secret in the target namespace before
running the installer, then supply `--tls-secret`:

```sh
./deploy/helm/vmcp/quickstart.sh --context YOUR_CONTEXT \
  --web-host console.example.com --gateway-host mcp.example.com \
  --tls-secret vmcp-public-tls --ingress-class nginx
```

For a **fresh install behind an external HTTPS proxy**, use `--external-https`
instead of `--tls-secret` (since v1.2.4):

```sh
./deploy/helm/vmcp/quickstart.sh --context microk8s --version v1.3.0 \
  --storage-class microk8s-hostpath --mock --omni --keycloak \
  --web-host console.example.com --gateway-host mcp.example.com \
  --keycloak-host sso.example.com --external-https \
  --ingress-class nginx --no-port-forward --timeout 20m
```

The external proxy must provide certificates and terminate HTTPS for all three
hosts. It must preserve the original host and correctly set/overwrite trusted
`X-Forwarded-*` headers. Forward traffic to the HTTP ingress. In this mode the
installer does not create ingress TLS entries, require a TLS Secret, or enable
nginx HTTPS redirects; enforce external HTTPS at the proxy.

All public URLs use HTTPS: console callback and allowed origin, gateway/Omni URLs,
Keycloak hostname, console/runtime issuer, and demo team issuers. The newly created
`vmcp-console` Keycloak client gets the exact HTTPS `/auth/callback` redirect URI
and HTTPS Web Origin. Internal discovery and Keycloak pod traffic remain HTTP.
`--external-https` requires both ingress hosts and cannot be combined with
`--tls-secret`. Without either option, public URLs remain HTTP.

This is not a migration option: existing Keycloak clients and team mappings remain
create-only and are not rewritten when flags change.

Ingress mode prints HTTP or HTTPS URLs according to these TLS options and does not
start port forwards. The default
streaming annotations target nginx; other controllers may need manual adjustment.
TLS on ingress does not make the trial's internal infrastructure production ready.

Advanced/offline release selection accepts `--chart PATH --images-file PATH`.
The JSON inventory must match `--version` and contain all expected digest-pinned
images. For a local chart directory the installer restores locked dependencies;
that requires chart repository access. An archive already contains them. These
options skip release asset downloading/checksum verification; use trusted local files.
Cluster nodes still need access to the specified container images.

## Reruns, troubleshooting and cleanup

Rerun with the same namespace/release and the desired complete set of options.
Existing encryption keys, admin tokens and infrastructure passwords are reused.
Reruns are upgrades; omitted optional workloads are disabled. Persistent data stays
in PVCs, and credentials stay in `<release>-quickstart`. Preserve that Secret with
your database: losing the master key prevents decrypting existing upstream secrets.
Do not manually change database passwords or switch persistent/ephemeral storage
on an existing trial. Run only one installer per namespace/release at a time.

If an install fails, it retains resources for inspection:

```sh
kubectl --context YOUR_CONTEXT -n vmcp-demo get pods,jobs,pvc
kubectl --context YOUR_CONTEXT -n vmcp-demo get events --sort-by=.lastTimestamp
kubectl --context YOUR_CONTEXT -n vmcp-demo logs POD_NAME --all-containers
helm --kube-context YOUR_CONTEXT -n vmcp-demo list --all
```

Uninstall while preserving Secrets and PVCs:

```sh
./deploy/helm/vmcp/quickstart.sh --context YOUR_CONTEXT --uninstall
```

Delete the trial's stored data and credentials explicitly:

```sh
./deploy/helm/vmcp/quickstart.sh --context YOUR_CONTEXT --uninstall --purge
```

Use the same `--namespace` and `--release` used for installation. Neither command
deletes the namespace or cluster. PVC deletion follows your StorageClass reclaim
policy; it may delete the underlying volumes. Failed Helm/wait commands print resource status and events. No automatic rollback or destructive
cleanup runs on installation failure.
