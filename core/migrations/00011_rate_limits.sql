-- +goose Up
-- Calls per minute allowed per agent key and per virtual server. NULL = unlimited.
ALTER TABLE agent_keys ADD COLUMN rate_limit_per_minute integer CHECK (rate_limit_per_minute > 0);
ALTER TABLE virtual_servers ADD COLUMN rate_limit_per_minute integer CHECK (rate_limit_per_minute > 0);

-- +goose Down
ALTER TABLE virtual_servers DROP COLUMN rate_limit_per_minute;
ALTER TABLE agent_keys DROP COLUMN rate_limit_per_minute;
