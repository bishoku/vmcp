# Conversation audit correlation

Conversation IDs associate calls with an application conversation, independently
of MCP transport sessions. Configure optional extraction rules on an **Agent key →
Edit → Audit correlation**. The Antigravity preset fills its conversation and root
conversation metadata keys. Turn ID is optional. No client change or fixed vMCP
metadata namespace is required.

Mappings are JSON Pointers relative to `tools/call.params._meta`. Escape `/` inside
a key as `~1`, and `~` as `~0`. Nested objects and array indices are supported.

```json
{
  "auditCorrelation": {
    "conversationId": "/antigravity.google~1conversation_id",
    "rootConversationId": "/antigravity.google~1root_conversation_id"
  }
}
```

Create or PATCH an agent key with this block. Omission on PATCH preserves the
mapping; `{ "auditCorrelation": {} }` disables it. Rotation preserves the mapping
and credential identity. Deleting/recreating a key creates a different identity
and therefore a different conversation group. These rules are currently agent-key
settings; OAuth bindings and anonymous requests continue without extraction.
Agent keys are not part of the existing declarative export/apply document.

A path is limited to 512 bytes and 16 segments. Extracted IDs must be nonempty
strings, at most 256 bytes, without control characters. Metadata extraction is
bounded to 32 KiB. Missing, malformed or oversized optional correlation data never
rejects a tool call; the audit correlation status is `missing` or `invalid`. This
is separate from upstream Caller context rules, which can still require fields.
No IDs are inferred from `progressToken`, JSON-RPC request IDs, or earlier calls.
A missing turn remains absent. A missing root falls back to conversation ID for
grouping only; the original root field is not fabricated.

## Direct virtual MCP and Omni

The same verified agent-key mapping is applied on direct virtual MCP tool calls
and all four Omni tools. For Omni invocation, outer `params._meta` is forwarded to
the private invocation bridge, then to the virtual tool call. It is not merged
into business arguments. Existing upstream Caller context configuration determines
which namespaced fields reach MCP upstream metadata or REST headers. Arbitrary
metadata and the caller's progress token are not automatically forwarded upstream.
Metadata is request-scoped and is never cached on a shared session/client.

Omni retains stateless client transport handling and creates a transient execution
session per invocation. Conversation IDs survive this execution boundary and are
inherited by virtual/upstream and discovery/provider audit children. This feature
does not add persistent Omni transport sessions, upstream connection reuse, or
progress notification relay. Existing direct MCP sessions are unchanged.

## Reading audit records

Use **Audit Log → Conversations** for database-aggregated summaries, then **View
operations** to inspect the existing operation trees. Conversation ID, root
conversation ID, turn ID and conversation group ID can also be filtered under
**Correlation filters** and appear in audit details.

A conversation group is a SHA-256 digest of a versioned, unambiguous tuple of the
immutable agent-key UUID and root ID (or conversation ID when root is absent).
Agent-key UUIDs belong to a tenant; queries retain tenant and management access
restrictions. Different credentials never merge solely because they report the
same ID. Direct and Omni calls can join when the same key and root are supplied.
Root and child conversation IDs remain separately searchable within that group.
These client-supplied labels are correlation hints, not verified identities,
authorization, ownership, replay protection or an audit integrity guarantee.

API: `GET /api/v1/audit-events?groupBy=conversation` returns summaries with counts
for authorized records matching the active filters. `operations` counts distinct
root operations, excluding explicitly linked children; `errors` counts failed or
denied root outcomes. Groups are paginated **after aggregation** by their latest
record ID. Concurrent writes can move active groups between pages; refresh for a
current view. Time/status/stage filters also narrow counts. Calls without IDs are
excluded from conversation summaries but remain in Operations/Raw events.

Exact filters: `conversationId`, `rootConversationId`, `turnId`,
`conversationGroupId`. Existing filters and permissions still apply. Audit exporters
include the new database metadata fields in their existing envelopes.

## Storage and upgrade

Extraction defaults off for existing/new keys. Enabling it stores the selected IDs
as searchable **audit metadata even when payload capture is off**. Use non-sensitive
opaque identifiers. Payload masking is separate and does not mask these indexed
correlation columns. Unselected fields (such as artifact filesystem paths) are not
promoted to indexed metadata; existing payload capture/masking still governs the
original request. Ordinary audit retention also covers correlation metadata.

Run migration **35 (`00035_audit_conversations.sql`)** before starting the updated
control/data planes and audit writers. It adds the key configuration, audit
columns and partial indexes. Existing rows retain empty correlation metadata; no
backfill is attempted. Existing clients without metadata continue unchanged. Back
up before upgrades. Rollback removes the new mapping/correlation columns and
indexes, losing their contents, while preserving existing key/audit rows.
