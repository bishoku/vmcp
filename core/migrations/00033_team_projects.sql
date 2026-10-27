-- +goose Up
CREATE TABLE teams (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), slug text NOT NULL UNIQUE,
 name text NOT NULL, issuer text NOT NULL, group_value text NOT NULL,
 version bigint NOT NULL DEFAULT 1, UNIQUE(issuer,group_value)
);
CREATE TABLE projects (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), slug text NOT NULL UNIQUE,
 name text NOT NULL, owner_team_id uuid REFERENCES teams(id) ON DELETE RESTRICT,
 version bigint NOT NULL DEFAULT 1
);
INSERT INTO projects(slug,name) VALUES('default','Default');
ALTER TABLE upstreams ADD COLUMN owner_team_id uuid REFERENCES teams(id) ON DELETE RESTRICT;
ALTER TABLE virtual_servers ADD COLUMN project_id uuid REFERENCES projects(id) ON DELETE RESTRICT;
UPDATE virtual_servers SET project_id=(SELECT id FROM projects WHERE slug='default');
ALTER TABLE virtual_servers ALTER COLUMN project_id SET NOT NULL;
-- Preserve inserts by older API clients and internal fixtures.
-- +goose StatementBegin
CREATE FUNCTION default_virtual_project() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.project_id IS NULL THEN SELECT id INTO NEW.project_id FROM projects WHERE slug='default'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER virtual_default_project BEFORE INSERT ON virtual_servers FOR EACH ROW EXECUTE FUNCTION default_virtual_project();
-- +goose StatementEnd
CREATE TABLE tool_grants (
 virtual_server_id uuid NOT NULL REFERENCES virtual_servers(id) ON DELETE CASCADE,
 upstream_id uuid NOT NULL REFERENCES upstreams(id) ON DELETE CASCADE,
 tool_name text NOT NULL,
 status text NOT NULL CHECK(status IN ('pending','approved','rejected','revoked')),
 reason text NOT NULL DEFAULT '', actor text NOT NULL DEFAULT '',
 version bigint NOT NULL DEFAULT 1, updated_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(virtual_server_id,upstream_id,tool_name)
);
CREATE INDEX virtual_project_idx ON virtual_servers(project_id);
CREATE INDEX upstream_owner_idx ON upstreams(owner_team_id);
-- +goose StatementBegin
CREATE TRIGGER teams_bump AFTER INSERT OR UPDATE OR DELETE ON teams FOR EACH STATEMENT EXECUTE FUNCTION bump_config_revision();
CREATE TRIGGER projects_bump AFTER INSERT OR UPDATE OR DELETE ON projects FOR EACH STATEMENT EXECUTE FUNCTION bump_config_revision();
CREATE TRIGGER tool_grants_bump AFTER INSERT OR UPDATE OR DELETE ON tool_grants FOR EACH STATEMENT EXECUTE FUNCTION bump_config_revision();
-- +goose StatementEnd

-- +goose Down
DROP TABLE tool_grants;
DROP TRIGGER virtual_default_project ON virtual_servers;
DROP FUNCTION default_virtual_project();
ALTER TABLE virtual_servers DROP COLUMN project_id;
ALTER TABLE upstreams DROP COLUMN owner_team_id;
DROP TABLE projects;
DROP TABLE teams;
