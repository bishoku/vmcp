-- +goose Up
-- Identity bindings map a verified IdP token (claims) to the virtual servers its holder may use.
CREATE TABLE identity_bindings (
    id                    uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id             uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    slug                  text NOT NULL,
    description           text NOT NULL DEFAULT '',
    conditions            jsonb NOT NULL,
    ip_allowlist          text[] NOT NULL DEFAULT '{}',
    rate_limit_per_minute integer CHECK (rate_limit_per_minute > 0),
    enabled               boolean NOT NULL DEFAULT true,
    version               bigint NOT NULL DEFAULT 1,
    created_at            timestamptz NOT NULL DEFAULT now(),
    updated_at            timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, slug)
);

CREATE TABLE identity_binding_servers (
    binding_id        uuid NOT NULL REFERENCES identity_bindings(id) ON DELETE CASCADE,
    virtual_server_id uuid NOT NULL REFERENCES virtual_servers(id) ON DELETE CASCADE,
    PRIMARY KEY (binding_id, virtual_server_id)
);

-- +goose StatementBegin
CREATE TRIGGER identity_bindings_bump AFTER INSERT OR UPDATE OR DELETE ON identity_bindings
    FOR EACH STATEMENT EXECUTE FUNCTION bump_config_revision();
CREATE TRIGGER identity_binding_servers_bump AFTER INSERT OR UPDATE OR DELETE ON identity_binding_servers
    FOR EACH STATEMENT EXECUTE FUNCTION bump_config_revision();
-- +goose StatementEnd

-- JWT subject of the caller (agent_key then holds "oauth:<binding slug>").
ALTER TABLE audit_events ADD COLUMN subject text;

-- +goose Down
ALTER TABLE audit_events DROP COLUMN subject;
DROP TRIGGER identity_binding_servers_bump ON identity_binding_servers;
DROP TRIGGER identity_bindings_bump ON identity_bindings;
DROP TABLE identity_binding_servers;
DROP TABLE identity_bindings;
