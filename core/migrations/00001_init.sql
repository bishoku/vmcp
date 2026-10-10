-- +goose Up
-- +goose StatementBegin

-- Every row is scoped to a tenant so multi-tenancy never needs a retrofit.
CREATE TABLE tenants (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    slug       text NOT NULL UNIQUE,
    name       text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

INSERT INTO tenants (slug, name) VALUES ('default', 'Default');

-- Physical MCP servers. Auth headers are stored encrypted (AES-GCM) by the control plane.
CREATE TABLE upstreams (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id          uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    slug               text NOT NULL,
    name               text NOT NULL,
    url                text NOT NULL,
    transport          text NOT NULL CHECK (transport IN ('streamable_http', 'sse')),
    encrypted_headers  bytea,
    enabled            boolean NOT NULL DEFAULT true,
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, slug)
);

-- Cached tool schemas fetched from each upstream.
CREATE TABLE upstream_tools (
    upstream_id   uuid NOT NULL REFERENCES upstreams(id) ON DELETE CASCADE,
    name          text NOT NULL,
    description   text NOT NULL DEFAULT '',
    input_schema  jsonb NOT NULL DEFAULT '{}'::jsonb,
    annotations   jsonb NOT NULL DEFAULT '{}'::jsonb,
    fetched_at    timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (upstream_id, name)
);

CREATE TABLE virtual_servers (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    slug        text NOT NULL,
    name        text NOT NULL,
    description text NOT NULL DEFAULT '',
    enabled     boolean NOT NULL DEFAULT true,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, slug)
);

-- Tool allowlist of a virtual server. alias overrides the namespaced name when set.
CREATE TABLE virtual_server_tools (
    virtual_server_id uuid NOT NULL REFERENCES virtual_servers(id) ON DELETE CASCADE,
    upstream_id       uuid NOT NULL,
    tool_name         text NOT NULL,
    alias             text,
    PRIMARY KEY (virtual_server_id, upstream_id, tool_name),
    FOREIGN KEY (upstream_id, tool_name) REFERENCES upstream_tools(upstream_id, name) ON DELETE CASCADE
);

-- Single-row revision counter. Bumped on every config change; drives snapshot sync.
CREATE TABLE config_revision (
    id       boolean PRIMARY KEY DEFAULT true CHECK (id),
    revision bigint NOT NULL DEFAULT 1
);
INSERT INTO config_revision DEFAULT VALUES;

-- +goose StatementEnd

-- +goose Down
DROP TABLE config_revision;
DROP TABLE virtual_server_tools;
DROP TABLE virtual_servers;
DROP TABLE upstream_tools;
DROP TABLE upstreams;
DROP TABLE tenants;
