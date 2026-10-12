# Getting started

This walkthrough uses the local demo with bundled MCP servers. You will register
one upstream, expose one tool and verify a connection using a separate agent key.
No Go installation or external MCP account is needed.

## Prerequisites

- Docker with Compose and roughly 6–8 GiB available memory for the full demo.
- Git and internet access for the first build.
- Available demo ports; see the [sandbox guide](../benchmark/README.md#urls-and-credentials).

For a smaller installation using your own HTTP MCP server, use
[Local Docker installation](Deployment.md#local-docker-installation).
For an existing development cluster, use [Kubernetes quickstart](HELM_QUICKSTART.md).

## 1. Start the demo

```sh
git clone https://github.com/bishoku/vmcp.git
cd vmcp
docker compose -f benchmark/compose.yaml up -d --build
docker compose -f benchmark/compose.yaml ps
curl -fsS http://localhost:8801/readyz
curl -fsS http://localhost:8802/readyz
curl -fsS http://localhost:8803/readyz
```

Wait for the migration service to complete and the planes to become ready.
Keycloak may take a minute on first boot. If a readiness request fails, inspect
`docker compose -f benchmark/compose.yaml logs` and retry when startup completes.

Open **http://localhost:3300**. Sign in using the admin token `demo-admin-token`.
The demo contains public development credentials and synthetic data; keep it local.
Configuration starts empty. Optional SSO users are listed in the sandbox guide.

## 2. Register and discover an upstream

Open **Upstreams** and create an upstream with these values:

| Setting | Value |
|---|---|
| Slug | `calculator` |
| Name | `Calculator` |
| URL | `http://mcp-calculator:9100/mcp` |
| Transport | Streamable HTTP |
| Header name | `Authorization` |
| Header value | `Bearer calculator-secret` |

Use the container service URL above: `localhost` inside vMCP would point to the
vMCP container itself. The demo already grants outbound access to its fixtures.
Save the upstream, test its stored connection, then refresh its capabilities.
You should see the calculator tools, including `calculate`.

## 3. Create a virtual server

Open **Virtual Servers → New virtual server**. Use slug `demo`, select the
calculator's `calculate` tool and give it alias `sum`. Keep the server enabled
and **authentication required** enabled. Save it.

The virtual server exposes only the capabilities you select. You can add
resources, templates and prompts later; see [Virtual servers](Virtual-Servers.md).

## 4. Grant access

Open **Agent keys** and create a key granting access to `demo`. Copy the secret
when it is displayed; it is shown only once. Keep it separate from the admin token:
the admin token manages vMCP, while the agent key connects to the virtual server.

Creating a key does not change a server's authentication setting. If you previously
created an anonymous server, edit it and enable **authentication required** first.

## 5. Connect and verify

Open the saved server's **Connect** view. Its endpoint should be:

```text
http://localhost:8800/mcp/v1/virtual/demo
```

Configure your MCP client for Streamable HTTP with this endpoint and header
`Authorization: Bearer <your-agent-key>`. Client configuration formats vary; use
its HTTP transport settings. vMCP does not provide a stdio executable for this connection.

In **Verify connection**, enter the agent key and run verification. Successful
discovery should list `sum`. Verification does not execute the tool. To test an
actual call, invoke `sum` deliberately in your MCP client using its advertised
input schema, then inspect the server's **Activity** or the **Audit Log**.

If verification fails, see [FAQ and troubleshooting](FAQ.md) and the
[browser connection requirements](Connections-and-Setup.md#deliberate-browser-verification).

## Stop or remove the demo

Stop containers while preserving configuration and credentials:

```sh
docker compose -f benchmark/compose.yaml down
```

To also delete the demo's stored data, including configuration and Keycloak data:

```sh
docker compose -f benchmark/compose.yaml down -v
```

Use the same `VMCP_DEMO_SUBNET` value for every command if you customized it.

## Next steps

- [Guided setup and connection details](Connections-and-Setup.md)
- [Tool policies](Tool-Policies.md) and [PII masking](PII-Masking.md)
- [Agent authentication](Agent-Keys-and-Authentication.md) and [SSO roles](SSO-and-RBAC.md)
- [Installation options](Deployment.md) and [production deployment](PRODUCTION_DEPLOYMENT.md)
