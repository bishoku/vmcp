-- +goose Up
ALTER TABLE virtual_servers ADD COLUMN client_features jsonb NOT NULL DEFAULT '{}';
ALTER TABLE upstream_tools ADD COLUMN definition jsonb NOT NULL DEFAULT '{}';
ALTER TABLE sessions ADD COLUMN logging_level text NOT NULL DEFAULT 'warning';
CREATE TABLE mcp_client_notifications (
 session_id text PRIMARY KEY REFERENCES sessions(id) ON DELETE CASCADE,
 roots_revision bigint NOT NULL DEFAULT 0
);
-- +goose Down
DROP TABLE mcp_client_notifications;
ALTER TABLE sessions DROP COLUMN logging_level;
ALTER TABLE upstream_tools DROP COLUMN definition;
ALTER TABLE virtual_servers DROP COLUMN client_features;
