# Your Company Has 100 AI Agents. Who Controls What They Can Do?

*Introducing vMCP: an open-source gateway for bringing some order to enterprise MCP connections.*

## When a useful experiment becomes a shared problem

Imagine a company called Northstar. It has a support team, a finance team, several engineering teams, and a growing interest in AI agents.

The first agent helps developers search internal documentation. It connects to one Model Context Protocol (MCP) server. Someone adds a credential to its configuration, and the experiment works well.

A few months later, the picture looks different.

Support agents look up customers and draft replies. Engineering agents read repositories and create issues. Finance agents check invoices. Other agents search policies, investigate incidents, or prepare reports. Several teams have built MCP servers around their own systems.

Now there are dozens of agents, multiple client applications, and many MCP endpoints.

Which agent can change a customer record? Where are the upstream credentials stored? Did a failed request reach the billing system? What happens when someone leaves the company? Can a support agent access the finance tools simply because they share a server?

Northstar is fictional, but it illustrates the problem vMCP is trying to address: connecting an agent to a tool is only the beginning. Managing those connections over time needs its own layer.

## A gateway with a smaller view for each agent

vMCP is an open-source gateway for MCP. Its main idea is a **virtual MCP server**.

An operator registers upstream MCP servers, discovers their capabilities, and selects which ones to expose through a virtual server. An agent connects to that virtual endpoint instead of managing every upstream connection itself.

At Northstar, a support virtual server could expose customer lookup, a troubleshooting resource, and a reply-writing prompt. An engineering virtual server could expose repository search and issue creation. Finance could have a separate endpoint with its own access rules.

Tools, resources, resource templates, and prompts are explicitly selected. Operators can rename supported objects and override descriptions to make the exposed catalog easier to understand.

An upstream credential stays with the gateway. The agent receives an agent key or uses a verified identity-provider token to access its allowed virtual servers.

This does introduce another service to operate. The hope is that one managed gateway is easier to reason about than a growing collection of independent credentials, policies, and connection settings.

## Design principle: make access deliberate

A virtual server is more than a convenient list of endpoints. It defines a particular view of the available capabilities.

Selecting a few tools does not automatically grant access to every resource or prompt on their upstream server. Those selections are managed separately.

Tool policies support allow, deny, and read-only modes. The read-only decision uses the upstream's declared `readOnlyHint`. That is useful, but it remains a declaration from the upstream. It is not proof that the implementation cannot change data.

The same care applies to interactive MCP features. Sampling, roots, and form elicitation require explicit virtual-server permissions and negotiation with the downstream client. Those permissions default to off.

For example, enabling sampling does not give an upstream the gateway's model credentials. The request is routed to the connected MCP host, which remains responsible for approval and model access.

The supported protocol baseline is MCP revision 2025-06-18. Within its documented scope, vMCP handles Streamable HTTP, SSE delivery, progress, cancellation, replay, subscriptions, and selected completion flows. Upstreams can use Streamable HTTP, legacy HTTP+SSE, or a separate isolated stdio worker.

## Design principle: separate management from requests

vMCP has a Go core and a React management interface. The backend separates two responsibilities.

The control plane manages configuration, credentials, policies, and administrative access through a REST API. The web interface uses that API. An OpenAPI 3.1 contract and declarative export/apply support also make automation possible.

The data plane accepts agent connections, checks access, applies configured controls, and communicates with upstream MCP servers.

For teams that prefer configuration review in Git, export and apply provide a starting point for a GitOps workflow. For teams exploring the system, the UI offers a guided connection journey: register an upstream, test it, discover capabilities, compose a virtual server, grant access, and connect.

The connection view shows relationships from configured identities through virtual servers to upstreams. It is a map of configuration, not a claim that every displayed identity is currently connected.

## Security needs several boundaries

vMCP includes security controls, but none removes the need to trust and review the systems around it.

Upstream headers and supported secrets are encrypted at rest with AES-256-GCM. Versioned encryption envelopes bind protected values to their record and purpose. The project also includes a restartable rotation command for supported encrypted fields.

The master key still needs careful custody. Encryption in PostgreSQL offers limited protection if an attacker also controls the running gateway and its keys. Key injection, backup access, and recovery procedures remain deployment responsibilities.

Agent keys are stored as SHA-256 hashes. Identity-provider authentication verifies asymmetric JWT signatures through JWKS and checks the configured issuer, audience, and validity periods. Identity bindings map verified claims to allowed virtual servers.

Administrative SSO supports roles, while shared browser-session records support revocation. An exact-token JWT denylist is available for containment, although its operational workflow currently requires controlled database access.

Network controls address another important boundary: an operator-supplied upstream URL must not become an unrestricted route into internal infrastructure. vMCP applies egress restrictions, and internal destinations require explicit grants. Upstream TLS supports custom certificate authorities and mutual TLS without disabling certificate verification.

PII masking can inspect supported content using built-in detectors and custom regular expressions. Under an enabled inspection policy, certain content that cannot be safely inspected is denied.

Pattern matching can still miss sensitive information. It also does not solve prompt injection. A resource or prompt can contain misleading instructions without containing a single recognizable personal identifier. Selected upstreams, host-side approval, and content trust boundaries still need review.

## Scaling out without promising unlimited scale

PostgreSQL is the shared source of truth for configuration and important operational records.

Configuration changes advance a revision. PostgreSQL LISTEN/NOTIFY wakes the data planes, while polling provides a fallback when a notification is missed. Nodes keep in-memory snapshots for use in request handling.

Authorization has a freshness limit. A node may retain its last good snapshot after a refresh failure, but it cannot keep using that snapshot to authorize requests indefinitely. Once confirmation becomes too old, authorization fails closed.

Shared session, operation, and replay records allow multiple gateway nodes to handle downstream sessions without requiring load-balancer affinity. Physical upstream connections and caches remain local to each node.

This is not an exactly-once execution guarantee. If an upstream changes something and the connection fails before the response arrives, retrying may repeat that change. Retries need defined idempotency rules, especially for tools with side effects.

Circuit breakers, failover URLs, health observations, and bounded request handling help manage upstream failures. Optional Redis provides shared rate-limit counters; the operator chooses the behavior when Redis is unavailable.

Adding data-plane replicas does not remove the database dependency. PostgreSQL capacity, availability, audit writes, and replay storage all affect practical scale. Prometheus metrics and optional OTLP tracing help operators observe the system, but representative load tests and recovery drills are still needed before making capacity promises.

## Audit records should explain uncertainty

In an enterprise, “the request failed” is often an incomplete answer.

Under strict audit mode, vMCP persists an admission record before a protected upstream operation is dispatched, then records its outcome. Administrative configuration changes and their audit records share a database transaction.

Records carry metadata such as identity, selected capability, configuration revision, timing, and correlation IDs. Arguments and results are excluded from the audit log, although identity metadata can still be personal data.

An admission without an outcome is incomplete evidence. It does not prove success or failure. The UI keeps raw events available and avoids presenting an unloaded outcome as a confirmed error.

A separate exporter supports durable HTTPS delivery to a SIEM or another evidence sink, with retries and event IDs for deduplication. Retention accounts for undelivered evidence and legal holds.

There are open operational gaps here, including restricted-role permissions and stronger separation of retention duties. PostgreSQL records are also editable by a sufficiently privileged database administrator. Organizations needing immutable evidence must establish a separate trusted sink.

These features can support an organization's controls. They do not certify SOC 2, ISO 27001, or GDPR compliance.

## What comes next

vMCP remains Beta, with APIs that may change. The next steps focus on proving reliable operation as well as adding features.

The production roadmap includes stronger stdio worker containment and crash recovery, corrected least-privilege audit identities, and more consistent policy and audit coverage across capability paths.

Audit operations need further work around retention separation, large operations, exporter backlog alerts, and sink authentication.

Deployment evidence is equally important: sustained multi-node load, PostgreSQL promotion, Redis outages, rolling restarts, and backup restoration with measured recovery targets. Tenant routing, complete image scanning, and artifact provenance also remain priorities.

Today, vMCP offers a place to curate MCP capabilities, manage access, protect credentials, and investigate activity. Its value will depend on how well those controls behave under real workloads and failures.

If this problem sounds familiar, the repository includes a local demonstration stack with PostgreSQL, Redis, Keycloak, two data planes, a gateway, and three synthetic MCP servers. It starts with an empty configuration so you can explore the setup through the UI.

The project is available under Apache-2.0:

[https://github.com/bishoku/vmcp](https://github.com/bishoku/vmcp)

Contributions are welcome: bug reports, documentation improvements, interoperability tests, operational feedback, and code. If you try vMCP, sharing what was confusing or what failed would help make the next version better.
