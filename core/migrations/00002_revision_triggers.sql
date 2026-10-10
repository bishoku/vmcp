-- +goose Up
-- +goose StatementBegin

-- Any change to config tables bumps the global revision and notifies listeners.
-- Doing it in the database means every writer (control plane, scripts, psql) is covered.
CREATE FUNCTION bump_config_revision() RETURNS trigger AS $$
DECLARE
    rev bigint;
BEGIN
    UPDATE config_revision SET revision = revision + 1 RETURNING revision INTO rev;
    PERFORM pg_notify('vmcp_config', rev::text);
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER tenants_bump AFTER INSERT OR UPDATE OR DELETE ON tenants
    FOR EACH STATEMENT EXECUTE FUNCTION bump_config_revision();
CREATE TRIGGER upstreams_bump AFTER INSERT OR UPDATE OR DELETE ON upstreams
    FOR EACH STATEMENT EXECUTE FUNCTION bump_config_revision();
CREATE TRIGGER upstream_tools_bump AFTER INSERT OR UPDATE OR DELETE ON upstream_tools
    FOR EACH STATEMENT EXECUTE FUNCTION bump_config_revision();
CREATE TRIGGER virtual_servers_bump AFTER INSERT OR UPDATE OR DELETE ON virtual_servers
    FOR EACH STATEMENT EXECUTE FUNCTION bump_config_revision();
CREATE TRIGGER virtual_server_tools_bump AFTER INSERT OR UPDATE OR DELETE ON virtual_server_tools
    FOR EACH STATEMENT EXECUTE FUNCTION bump_config_revision();

-- +goose StatementEnd

-- +goose Down
DROP TRIGGER virtual_server_tools_bump ON virtual_server_tools;
DROP TRIGGER virtual_servers_bump ON virtual_servers;
DROP TRIGGER upstream_tools_bump ON upstream_tools;
DROP TRIGGER upstreams_bump ON upstreams;
DROP TRIGGER tenants_bump ON tenants;
DROP FUNCTION bump_config_revision();
