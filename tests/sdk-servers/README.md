# Upstream SDK interoperability

The matrix checks Streamable HTTP discovery and synchronous calls for protocol revisions 2025-06-18 and 2026-07-28. Each server exposes one echo tool. A separate client tests both revisions directly on the same URL before the control plane registers the upstream. Each of the four downstream cases invokes the published alias twice and checks the native callback count.

| Server | Legacy profile | Modern profile | Dual profile |
| --- | --- | --- | --- |
| Go SDK 1.8.0 | Legacy revision, stateful | Modern revision, stateless | Both revisions |
| TypeScript SDK | 1.32.1 Streamable HTTP | 2.4.0 with legacy rejected | 2.4.0 with stateless legacy support |
| Python FastMCP | 3.4.11, mcp 1.30.0 | 4.2.2, mcp 2.3.0 with an ASGI revision-header requirement | 4.2.2, mcp 2.3.0 with default negotiation |

The FastMCP modern-only row exercises a deployment restriction. FastMCP 4 supplies the native dual handler. A small ASGI wrapper rejects requests lacking the modern revision header before they reach that handler. The test does not establish a native FastMCP modern-only setting. The Go direct check verifies whether the SDK agrees to the requested legacy revision; an initialize response offering a different revision counts as rejection of that revision.

## Running locally

Use a disposable PostgreSQL database. Export its URL as `VMCP_TEST_DATABASE_URL` and set `VMCP_REQUIRE_INTEGRATION=1`. From the repository root, with Go, Node.js and uv installed, run:

```sh
npm ci --prefix web
uv sync --locked --project tests/sdk-servers/python
uv sync --locked --project tests/sdk-servers/python-legacy
export VMCP_GO_SDK_SERVER="$(mktemp -d)/upstream-go"
(cd tests/sdk-servers/go && go build -mod=readonly -o "$VMCP_GO_SDK_SERVER" .)
(cd core && go test -race -count=1 -tags sdkinterop -run '^TestRealUpstream' -json ./internal/controlplane) > /tmp/sdk-results.jsonl
python3 tests/sdk-servers/report.py /tmp/sdk-results.jsonl
```

The transition test runs two independently cached gateway instances against the same database and waits for the production five-minute preference lifetime. CI retains the test event stream with method sequences, protocol headers, session presence and callback counts.
