-- +goose Up
ALTER TABLE upstreams ADD COLUMN kind text NOT NULL DEFAULT 'mcp' CHECK (kind IN ('mcp','rest'));
ALTER TABLE upstreams ADD COLUMN encrypted_rest_config bytea;
-- +goose Down
ALTER TABLE upstreams DROP COLUMN encrypted_rest_config;
ALTER TABLE upstreams DROP COLUMN kind;
