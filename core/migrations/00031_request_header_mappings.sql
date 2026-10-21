-- +goose Up
ALTER TABLE upstreams ADD COLUMN request_header_mappings jsonb NOT NULL DEFAULT '[]'::jsonb;

-- +goose Down
ALTER TABLE upstreams DROP COLUMN request_header_mappings;
