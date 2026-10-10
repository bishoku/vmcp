-- +goose Up
-- Named, revocable credentials for the admin API (the static VMCP_ADMIN_TOKEN stays as bootstrap).
-- No revision trigger: only the control plane reads this table.
CREATE TABLE admin_tokens (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    slug         text NOT NULL,
    description  text NOT NULL DEFAULT '',
    scope        text NOT NULL CHECK (scope IN ('admin', 'read')),
    token_hash   bytea NOT NULL UNIQUE,
    token_prefix text NOT NULL,
    expires_at   timestamptz,
    revoked_at   timestamptz,
    last_used_at timestamptz,
    version      bigint NOT NULL DEFAULT 1,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, slug)
);

-- +goose Down
DROP TABLE admin_tokens;
