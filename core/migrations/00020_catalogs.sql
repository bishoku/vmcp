-- +goose Up
ALTER TABLE upstreams ADD COLUMN mcp_capabilities jsonb NOT NULL DEFAULT '{}';
CREATE TABLE upstream_catalog (
 upstream_id uuid NOT NULL REFERENCES upstreams(id) ON DELETE CASCADE,
 kind text NOT NULL CHECK(kind IN ('resource','template','prompt')),
 name text NOT NULL CHECK(octet_length(name)<=4096),
 definition jsonb NOT NULL CHECK(jsonb_typeof(definition)='object'),
 PRIMARY KEY(upstream_id,kind,name)
);
CREATE TABLE virtual_catalog (
 virtual_server_id uuid NOT NULL REFERENCES virtual_servers(id) ON DELETE CASCADE,
 upstream_id uuid NOT NULL REFERENCES upstreams(id) ON DELETE CASCADE,
 kind text NOT NULL CHECK(kind IN ('resource','template','prompt')),
 name text NOT NULL CHECK(octet_length(name)<=4096),
 alias text NOT NULL DEFAULT '',
 description text NOT NULL DEFAULT '',
 description_basis text NOT NULL DEFAULT '',
 position integer NOT NULL,
 PRIMARY KEY(virtual_server_id,upstream_id,kind,name)
);
CREATE TRIGGER upstream_catalog_revision AFTER INSERT OR UPDATE OR DELETE ON upstream_catalog FOR EACH STATEMENT EXECUTE FUNCTION bump_config_revision();
CREATE TRIGGER virtual_catalog_revision AFTER INSERT OR UPDATE OR DELETE ON virtual_catalog FOR EACH STATEMENT EXECUTE FUNCTION bump_config_revision();
CREATE TABLE mcp_resource_subscriptions (
 session_id text NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
 uri text NOT NULL,
 upstream_id uuid NOT NULL REFERENCES upstreams(id) ON DELETE CASCADE,
 original_uri text NOT NULL,
 revision bigint NOT NULL,
 owner uuid,
 lease_until timestamptz,
 ready boolean NOT NULL DEFAULT false,
 failed boolean NOT NULL DEFAULT false,
 expires_at timestamptz NOT NULL DEFAULT now()+interval '15 minutes',
 PRIMARY KEY(session_id,uri)
);
-- +goose Down
DROP TABLE mcp_resource_subscriptions;
DROP TABLE virtual_catalog;
DROP TABLE upstream_catalog;
ALTER TABLE upstreams DROP COLUMN mcp_capabilities;
