-- +goose Up
ALTER TABLE virtual_servers ADD COLUMN guidance jsonb NOT NULL DEFAULT '{}';
ALTER TABLE upstreams ADD COLUMN discovered_instructions text NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE upstreams DROP COLUMN discovered_instructions;
ALTER TABLE virtual_servers DROP COLUMN guidance;
