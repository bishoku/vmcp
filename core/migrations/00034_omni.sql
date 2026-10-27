-- +goose Up
ALTER TABLE virtual_servers ADD COLUMN omni_enabled boolean NOT NULL DEFAULT false;
-- +goose Down
ALTER TABLE virtual_servers DROP COLUMN omni_enabled;
