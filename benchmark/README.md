# Local vMCP manual testing stack

A UI-first sandbox with PostgreSQL, Redis, Keycloak, one control plane, **two data
planes behind round-robin nginx**, the web UI and **three synthetic MCP servers**.
No upstreams, virtual servers, agent keys or identity bindings are pre-created.
You configure those from the UI. Keycloak users/clients are imported for convenience.

This directory is named `benchmark` for repeatable experiments; it is **not a
production benchmark or production deployment**. Images, HTTP, known credentials,
database-owner application access and synthetic data are development choices.
Published ports bind to `127.0.0.1`. Docker needs roughly 4–6 GiB available memory;
first builds need internet access. Python 3.10+ is needed only for host-side helpers.

## Start

From the repository root:

```sh
docker compose -f benchmark/compose.yaml up -d --build
docker compose -f benchmark/compose.yaml ps
```

The migration service must finish successfully before either plane starts.
Keycloak can take a minute on first boot. Check readiness explicitly:

```sh
curl -fsS http://localhost:8801/readyz
curl -fsS http://localhost:8802/readyz
curl -fsS http://localhost:8803/readyz
```

The project name is `vmcp-demo`, separate from `deploy/docker-compose.yml` and its
volumes. The dedicated subnet is `172.30.42.0/24`; only that CIDR and the exact
Keycloak/fixture hostnames are granted outbound access. If the subnet overlaps
another Docker network, set `VMCP_DEMO_SUBNET=172.30.43.0/24` consistently for every
Compose command, then recreate this project's containers. No hosts-file edit is
needed: the browser uses localhost, and dynamic Keycloak backchannel discovery
uses the internal service name.

## URLs and credentials

| Service | Host URL / connection | Credentials |
|---|---|---|
| Web UI | http://localhost:3300 | SSO users below, or admin token `demo-admin-token` |
| Gateway | `http://localhost:8800/mcp/v1/virtual/{slug}` | Agent key / IdP JWT when auth required |
| Control-plane API | http://localhost:8801/api/v1 | `Authorization: Bearer demo-admin-token` |
| Data plane 1 | `http://localhost:8802/mcp/v1/virtual/{slug}` | Same configured credential |
| Data plane 2 | `http://localhost:8803/mcp/v1/virtual/{slug}` | Same configured credential |
| Keycloak admin | http://localhost:9180/admin | `admin` / `admin` |
| PostgreSQL | `localhost:55432`, database `vmcp` | `vmcp` / `vmcp-demo` |
| Redis | `localhost:56379`, database 0 | No password in this local stack |
| Calculator MCP | http://localhost:9111/mcp | `Bearer calculator-secret` |
| Knowledge MCP | http://localhost:9112/mcp | `Bearer knowledge-secret` |
| Support MCP | http://localhost:9113/mcp | `Bearer support-secret` |

Use **localhost**, rather than 127.0.0.1, in browser URLs: the imported OIDC issuer
and callback are fixed to localhost. Keycloak realm `vmcp` persists in its separate
PostgreSQL database. All secrets here are intentionally public demo values.

| SSO username | Password | vMCP role |
|---|---|---|
| alice | alice | Admin |
| bob | bob | Editor |
| carol | carol | Viewer |
| dave | dave | No mapped group; sign-in refused |

SSO and agent JWTs are separate flows. `alice` manages the gateway; the
`billing-bot` client below authenticates MCP agents.

## 1. Register the three upstreams in the UI

Open the UI and sign in as Alice or use the admin token. In **Upstreams**, create:

| Suggested slug | URL entered in vMCP | Transport | Header |
|---|---|---|---|
| calculator | `http://mcp-calculator:9100/mcp` | Streamable HTTP | `Authorization: Bearer calculator-secret` |
| knowledge | `http://mcp-knowledge:9100/mcp` | Streamable HTTP | `Authorization: Bearer knowledge-secret` |
| support | `http://mcp-support:9100/mcp` | Streamable HTTP | `Authorization: Bearer support-secret` |

Use internal service URLs in the UI: `localhost` inside a plane container refers
to that plane, not the host or fixture. Save and **Refresh tools** for each upstream;
this also discovers resources/templates/prompts. Each server has five tools,
two resources, one template and one prompt. The stored connection test should
succeed with the saved header and fail with an incorrect secret.

The three servers deliberately share `demo://guide`, `demo://status`,
`demo://records/{id}` and prompt name `review`. vMCP namespaces selected objects;
physical URI/name collisions should not expose the wrong upstream.

## 2. Create a virtual server

In **Virtual Servers**, create slug `demo` and select, for example:

- `calculator.calculate`, with alias `sum`; `calculator.delay`, alias `slow`; `calculator.publish_update`, alias `publish`; optionally `calculator.ask_host`, alias `ask_host`.
- `knowledge.search_docs`, alias `search`; `support.get_customer`, alias `customer`.
- The `demo://guide` resources from all three upstreams. Give them aliases `calculator_guide`, `knowledge_guide`, `support_guide`, or keep generated names.
- Calculator's `demo://status`, alias `calculator_status`, and the `demo://records/{id}` template.
- All three `review` prompts, with distinct aliases such as `calculator_review`, `knowledge_review`, `support_review`.

Keep **authentication required** enabled and create an **Agent Key** granting
access to `demo`. Copy the key when it is shown. Leave IP allowlists empty for the
first test. The nginx proxy forwards client addresses and both planes trust only
the demo network. Turn on the Roots permission only when testing `ask_host` below.

The UI is a management UI, not an MCP conversation host. Use your MCP client or
the Python helper to invoke the virtual server:

```sh
export DEMO_KEY='vmcp_...paste-the-key...'
python3 benchmark/client.py --url http://localhost:8800/mcp/v1/virtual/demo \
  --token "$DEMO_KEY"
python3 benchmark/client.py --url http://localhost:8800/mcp/v1/virtual/demo \
  --token "$DEMO_KEY" --method tools/call \
  --params '{"name":"sum","arguments":{"a":4,"b":7,"operation":"add"}}'
```

The helper performs initialize/initialized, handles JSON and POST SSE, and deletes
its session afterward. Each invocation starts a new session. It is a small demo
client, not a full SDK, replay tester or conformance suite.

## 3. Resources, prompts, templates and completion

```sh
python3 benchmark/client.py --url http://localhost:8800/mcp/v1/virtual/demo \
  --token "$DEMO_KEY" --method resources/list
python3 benchmark/client.py --url http://localhost:8800/mcp/v1/virtual/demo \
  --token "$DEMO_KEY" --method resources/read --params '{"uri":"vmcp://calculator/alias/calculator_guide"}'
python3 benchmark/client.py --url http://localhost:8800/mcp/v1/virtual/demo \
  --token "$DEMO_KEY" --method resources/templates/list
python3 benchmark/client.py --url http://localhost:8800/mcp/v1/virtual/demo \
  --token "$DEMO_KEY" --method prompts/get \
  --params '{"name":"knowledge_review","arguments":{"subject":"security"}}'
python3 benchmark/client.py --url http://localhost:8800/mcp/v1/virtual/demo \
  --token "$DEMO_KEY" --method completion/complete \
  --params '{"ref":{"type":"ref/prompt","name":"knowledge_review"},"argument":{"name":"subject","value":"sec"}}'
```

Resource aliases are wrapped in a routed URI: alias `calculator_guide` on upstream
`calculator` becomes `vmcp://calculator/alias/calculator_guide`. Always copy the
exact `uri` from resources/list if your slugs/aliases differ. For templates, copy the exposed
`uriTemplate` from resources/templates/list, replace `{id}` with `C001`, then call
resources/read with the expanded URI. Physical `demo://...` identifiers should
not work through the virtual endpoint; use its exposed `vmcp://...` identifier.

## 4. Progress and resource subscriptions

The delay tool also honors `notifications/cancelled` with its active request ID;
use a capable MCP host to exercise explicit cancellation.

Stream progress through a POST SSE call. The fixture also emits a synthetic logging
notification; vMCP replaces the raw log data with its sanitized omission notice:

```sh
python3 benchmark/client.py --url http://localhost:8800/mcp/v1/virtual/demo \
  --token "$DEMO_KEY" --method tools/call \
  --params '{"name":"slow","arguments":{"seconds":5},"_meta":{"progressToken":"manual-progress"}}'
```

In terminal A, subscribe and open GET SSE for up to about 40 seconds:

```sh
python3 benchmark/client.py --url http://localhost:8800/mcp/v1/virtual/demo \
  --token "$DEMO_KEY" --watch vmcp://calculator/alias/calculator_status --seconds 40
```

In terminal B, call `publish` after the subscription is open:

```sh
python3 benchmark/client.py --url http://localhost:8800/mcp/v1/virtual/demo \
  --token "$DEMO_KEY" --method tools/call --params '{"name":"publish","arguments":{}}'
```

Expect `notifications/resources/updated` for the exposed URI. Read the resource
again to see the incremented revision. Updates are hints, not an exactly-once feed;
a physical watcher reconnect can miss a hint. The helper closes the watch on the
next received line after the deadline, so keepalive timing can extend it slightly.

## 5. PII, policy, quota, authentication and UI checks

| Test | Action | Expected result |
|---|---|---|
| PII | Call `customer` with `{"customer_id":"C001"}`, enable email/phone/US SSN masking, call again | Synthetic PII is redacted in text and structured content; compare Audit redactions |
| Tool policy | Set `publish` to denied or the server default to read-only | Destructive/non-read-only tool disappears or cannot execute |
| Errors | Select a fixture's `fail` tool and call it | Synthetic tool error is visible without credentials |
| Authentication | Omit/revoke the agent key while auth is required | Requests fail; revocation converges within the documented freshness bound |
| Quota | Set a low server/key limit and call repeatedly, alternating ports 8802/8803 | Shared Redis quota rejects across both nodes |
| Strict Redis outage | `docker compose -f benchmark/compose.yaml stop redis` | Readiness/quota checks fail; restart with `start redis` |
| Node loss | `docker compose -f benchmark/compose.yaml stop data-plane-1` | Calls routed to the stopped node can fail; new requests can reach the other node; unsafe POSTs are never transparently retried |
| Recovery | `docker compose -f benchmark/compose.yaml start data-plane-1` | Node becomes ready; configuration and session truth remain shared |
| Concurrency | Open the same virtual server in two tabs, save one, then save stale data in the other | ETag conflict requires reload |
| GitOps | Settings → Configuration as code → Export, edit, Preview, Apply | Secret values are omitted; prune requires explicit confirmation |
| SSO roles | Separate browser/private sessions for Bob, Carol, Dave | Editor/viewer controls and unmapped-user refusal |
| Audit | Inspect Audit after calls and configuration changes | Admission/outcome/mutation records; some early catalog denials remain a roadmap gap |

The gateway intentionally disables nginx upstream retries. Its static upstream
DNS is resolved when nginx starts. If you recreate data-plane containers (new
IPs), restart the gateway as well:

```sh
docker compose -f benchmark/compose.yaml restart gateway
```

## 6. Keycloak JWTs for agents

In **Identities**, create binding `billing-demo`:

- Condition: claim `azp`, operation `equals`, value `billing-bot`.
- Virtual servers: `demo`; enabled; initially no IP restriction.

Obtain a client-credentials JWT from the imported client and use it instead of an
agent key:

```sh
DEMO_JWT=$(curl -fsS http://localhost:9180/realms/vmcp/protocol/openid-connect/token \
  -d grant_type=client_credentials -d client_id=billing-bot -d client_secret=billing-secret \
  | python3 -c 'import json,sys; print(json.load(sys.stdin)["access_token"])')
python3 benchmark/client.py --url http://localhost:8800/mcp/v1/virtual/demo \
  --token "$DEMO_JWT" --method tools/list
```

Issuer is `http://localhost:9180/realms/vmcp`, audience `vmcp-gateway`. A valid JWT
without a matching server grant is denied. The realm also includes an example
`vmcp-gateway` OAuth client (`gateway-secret`) for separate upstream-OAuth experiments;
the three fixtures in this stack use static bearer headers, not OAuth validation.

## 7. Negotiated host features

Enable **Roots** on the virtual server, select/alias calculator's `ask_host`, and run:

```sh
python3 benchmark/client.py --url http://localhost:8800/mcp/v1/virtual/demo \
  --token "$DEMO_KEY" --host-roots --method tools/call \
  --params '{"name":"ask_host","arguments":{"kind":"roots"}}'
```

The fixture requests roots/list through vMCP; this opt-in helper returns an empty
list and never reads host files. To test sampling or form elicitation, use an MCP
host that implements the matching capability and approval UI; enable the matching
virtual-server permission, then invoke `ask_host` with kind `sampling` or
`elicitation`. The Python helper does not fabricate model output or user consent.

## 8. Metrics, persistence and troubleshooting

Both nodes expose metrics independently:

```sh
curl -fsS http://localhost:8802/metrics
curl -fsS http://localhost:8803/metrics
docker compose -f benchmark/compose.yaml logs -f control-plane data-plane-1 data-plane-2
```

Check health or a fixture directly (with its bearer token using the helper) before
investigating gateway selection/routing. Common causes: wrong internal URL, missing
Authorization header, undiscovered catalog, unselected resource/prompt, missing
agent grant or expired token. Do not broaden the egress policy to all private
networks to fix a typo.

PostgreSQL holds vMCP configuration, audit/session/delivery records and Keycloak's
separate database. Redis AOF is persisted for local restart experiments. This is
one PostgreSQL server, so it demonstrates gateway replication, **not database HA**.
The stack does not provision the separate stdio host/rootless daemon, tenant router,
HTTPS SIEM sink, external immutable archive or OTLP/Grafana infrastructure. Their
controls and remaining production work are documented in the main [roadmap](../README.md#roadmap-to-production-readiness).

Realm import runs on an empty realm. Editing the JSON does not overwrite an
existing Keycloak realm; use its admin console or deliberately reset demo volumes.

## Verification helpers and cleanup

```sh
# Fixture regressions: catalogs, auth, sessions, completion, progress, subscriptions and host replies
python3 -m unittest discover -s benchmark/mcp -v
# Optional full-stack smoke: creates uniquely named objects, tests, then deletes only those objects
python3 benchmark/smoke.py
```

The smoke test checks all three catalogs, curated namespace collisions, tools,
prompt completion, POST progress, GET subscription hints, host roots relay,
agent-key and Keycloak JWT auth, and the same session through both nodes. It leaves
synthetic audit records, as deletion of test configuration must not erase evidence.
Normal startup never runs it. Interrupted smoke runs may leave `smoke-*` objects;
review and remove those from the UI if necessary.

```sh
# Stop and remove this project's containers; keep your configuration and Keycloak realm
docker compose -f benchmark/compose.yaml down
# Destructive reset of THIS demo's database/Redis volumes only
docker compose -f benchmark/compose.yaml down -v
```

Keycloak configuration follows its [container import](https://www.keycloak.org/server/containers)
and [hostname/backchannel](https://www.keycloak.org/server/hostname) documentation.
