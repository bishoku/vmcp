-- +goose Up
ALTER TABLE upstreams ADD COLUMN caller_context jsonb NOT NULL DEFAULT '{}'::jsonb;
-- +goose Down
ALTER TABLE upstreams DROP COLUMN caller_context;
