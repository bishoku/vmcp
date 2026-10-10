-- +goose Up
-- Masking policy per virtual server (see internal/pii.Config). Empty object = disabled.
ALTER TABLE virtual_servers ADD COLUMN pii_config jsonb NOT NULL DEFAULT '{}'::jsonb;

-- +goose Down
ALTER TABLE virtual_servers DROP COLUMN pii_config;
