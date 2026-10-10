-- +goose Up
-- Tool policy per virtual server (see internal/policy.Config). Empty object = allow everything.
ALTER TABLE virtual_servers ADD COLUMN tool_policy jsonb NOT NULL DEFAULT '{}'::jsonb;

-- +goose Down
ALTER TABLE virtual_servers DROP COLUMN tool_policy;
