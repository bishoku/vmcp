-- +goose Up
-- +goose StatementBegin

-- Agents authenticate to virtual servers with API keys. Only a SHA-256 hash is stored
-- (keys are long random strings, so a fast hash is appropriate and allows indexed lookup).
ALTER TABLE virtual_servers ADD COLUMN auth_required boolean NOT NULL DEFAULT true;

CREATE TABLE agent_keys (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    slug         text NOT NULL,
    description  text NOT NULL DEFAULT '',
    key_hash     bytea NOT NULL UNIQUE,
    key_prefix   text NOT NULL,
    ip_allowlist text[] NOT NULL DEFAULT '{}',
    expires_at   timestamptz,
    revoked_at   timestamptz,
    version      bigint NOT NULL DEFAULT 1,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, slug)
);

-- Which virtual servers a key may use.
CREATE TABLE agent_key_servers (
    agent_key_id      uuid NOT NULL REFERENCES agent_keys(id) ON DELETE CASCADE,
    virtual_server_id uuid NOT NULL REFERENCES virtual_servers(id) ON DELETE CASCADE,
    PRIMARY KEY (agent_key_id, virtual_server_id)
);

CREATE TRIGGER agent_keys_bump AFTER INSERT OR UPDATE OR DELETE ON agent_keys
    FOR EACH STATEMENT EXECUTE FUNCTION bump_config_revision();
CREATE TRIGGER agent_key_servers_bump AFTER INSERT OR UPDATE OR DELETE ON agent_key_servers
    FOR EACH STATEMENT EXECUTE FUNCTION bump_config_revision();

-- +goose StatementEnd

-- +goose Down
DROP TRIGGER agent_key_servers_bump ON agent_key_servers;
DROP TRIGGER agent_keys_bump ON agent_keys;
DROP TABLE agent_key_servers;
DROP TABLE agent_keys;
ALTER TABLE virtual_servers DROP COLUMN auth_required;
