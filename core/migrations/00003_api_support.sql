-- +goose Up
-- +goose StatementBegin

-- ETag support: every write that changes a resource bumps its version.
ALTER TABLE upstreams ADD COLUMN version bigint NOT NULL DEFAULT 1;
ALTER TABLE upstreams ADD COLUMN description text NOT NULL DEFAULT '';
ALTER TABLE virtual_servers ADD COLUMN version bigint NOT NULL DEFAULT 1;

-- Agents should see tools in the order the operator chose.
ALTER TABLE virtual_server_tools ADD COLUMN position integer NOT NULL DEFAULT 0;

-- A selection must survive its tool disappearing upstream (it is reported as an
-- "orphaned selection" and simply not served until the tool is back). Keep the
-- cascade on the upstream, drop the one on the tool.
ALTER TABLE virtual_server_tools DROP CONSTRAINT virtual_server_tools_upstream_id_tool_name_fkey;
ALTER TABLE virtual_server_tools
    ADD CONSTRAINT virtual_server_tools_upstream_id_fkey
    FOREIGN KEY (upstream_id) REFERENCES upstreams(id) ON DELETE CASCADE;

-- Refresh bookkeeping lives apart from upstreams on purpose: it has no revision
-- trigger, so recording a refresh never forces the data plane to reload.
CREATE TABLE upstream_status (
    upstream_id        uuid PRIMARY KEY REFERENCES upstreams(id) ON DELETE CASCADE,
    last_refresh_at    timestamptz,
    last_refresh_error text
);

-- +goose StatementEnd

-- +goose Down
DROP TABLE upstream_status;
ALTER TABLE virtual_server_tools DROP CONSTRAINT virtual_server_tools_upstream_id_fkey;
DELETE FROM virtual_server_tools vt
 WHERE NOT EXISTS (SELECT 1 FROM upstream_tools ut WHERE ut.upstream_id = vt.upstream_id AND ut.name = vt.tool_name);
ALTER TABLE virtual_server_tools
    ADD CONSTRAINT virtual_server_tools_upstream_id_tool_name_fkey
    FOREIGN KEY (upstream_id, tool_name) REFERENCES upstream_tools(upstream_id, name) ON DELETE CASCADE;
ALTER TABLE virtual_server_tools DROP COLUMN position;
ALTER TABLE virtual_servers DROP COLUMN version;
ALTER TABLE upstreams DROP COLUMN description;
ALTER TABLE upstreams DROP COLUMN version;
