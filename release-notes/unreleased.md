# Unreleased

## Breaking change to REST tool arguments

REST operations with an object request body expose its top-level fields directly.
For an operation with path argument `id` and body field `text`, update calls from
`{"id":"7","body":{"text":"Ready"}}` to `{"id":"7","text":"Ready"}`. The HTTP body
remains `{"text":"Ready"}`. Nested fields keep their structure. A root array uses
one required argument named `body` by default. Set **Array argument name** in
the operation editor to choose another name.

1. Before upgrading, review saved operations for collisions between path, query
   (including fixed parameters) and top-level body names. Resolve collisions and
   schemas with ambiguous root types. Review whole-object constraints on bodies
   combined with URL parameters as described in [Upstreams](../docs/Upstreams.md).
2. Pause tool traffic and upgrade the control plane and all data-plane replicas.
3. Open each existing REST upstream and choose **Refresh Tools**, or POST to
   `/api/v1/upstreams/{slug}/refresh-tools`. This regenerates saved tool schemas
   locally and preserves virtual server selections and aliases.
4. Reload the client's tool list, update saved calls, and verify an example with
   **Preview request** before resuming traffic.
