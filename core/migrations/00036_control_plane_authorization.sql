-- +goose Up
-- Foundation only: existing authentication and ownership remain authoritative.
CREATE TABLE authorization_state (
 singleton boolean PRIMARY KEY DEFAULT true CHECK(singleton),
 revision bigint NOT NULL DEFAULT 1 CHECK(revision > 0),
 mode text NOT NULL DEFAULT 'legacy' CHECK(mode IN ('legacy','shadow','enforced'))
);
INSERT INTO authorization_state(singleton) VALUES(true);
CREATE TABLE authorization_permissions (
 id text PRIMARY KEY,
 scopes text[] NOT NULL CHECK(cardinality(scopes)>0),
 capture_restricted boolean NOT NULL DEFAULT false
);
CREATE TABLE authorization_roles (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 slug text NOT NULL UNIQUE CHECK(slug<>''), name text NOT NULL CHECK(name<>''),
 description text NOT NULL DEFAULT '', builtin boolean NOT NULL DEFAULT false,
 version bigint NOT NULL DEFAULT 1 CHECK(version>0)
);
CREATE TABLE authorization_role_permissions (
 role_id uuid NOT NULL REFERENCES authorization_roles(id) ON DELETE CASCADE,
 permission_id text NOT NULL REFERENCES authorization_permissions(id) ON DELETE RESTRICT,
 PRIMARY KEY(role_id,permission_id)
);
CREATE TABLE authorization_mappings (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 issuer text NOT NULL CHECK(issuer<>''), principal_kind text NOT NULL CHECK(principal_kind='oidc'),
 conditions jsonb NOT NULL CHECK(jsonb_typeof(conditions)='array' AND jsonb_array_length(conditions)>0),
 enabled boolean NOT NULL DEFAULT true, version bigint NOT NULL DEFAULT 1 CHECK(version>0)
);
CREATE TABLE authorization_assignments (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 role_id uuid NOT NULL REFERENCES authorization_roles(id) ON DELETE RESTRICT,
 principal_kind text CHECK(principal_kind IN ('oidc','control_token')),
 issuer text NOT NULL DEFAULT '', principal_id text,
 mapping_id uuid REFERENCES authorization_mappings(id) ON DELETE RESTRICT,
 scope_kind text NOT NULL CHECK(scope_kind IN ('tenant','unmanaged','team','project','upstream','virtual_server')),
 scope_id uuid, epoch_id uuid, owner_team_id uuid,
 capture_scopes text[] NOT NULL DEFAULT '{}',
 enabled boolean NOT NULL DEFAULT true, version bigint NOT NULL DEFAULT 1 CHECK(version>0),
 CHECK ((mapping_id IS NOT NULL AND principal_kind IS NULL AND principal_id IS NULL AND issuer='') OR
        (mapping_id IS NULL AND principal_kind IS NOT NULL AND principal_id IS NOT NULL AND principal_id<>'' AND
         ((principal_kind='oidc' AND issuer<>'') OR (principal_kind='control_token' AND issuer='')))),
 CHECK ((scope_kind IN ('tenant','unmanaged') AND scope_id IS NULL AND epoch_id IS NULL AND owner_team_id IS NULL) OR
        (scope_kind NOT IN ('tenant','unmanaged') AND scope_id IS NOT NULL)),
 CHECK(capture_scopes <@ ARRAY['virtual','upstream','control','reads','system','omni','omni_diagnostics','omni_embedding','omni_rerank']::text[])
);
CREATE INDEX authorization_assignments_principal ON authorization_assignments(principal_kind,issuer,principal_id) WHERE enabled;
CREATE INDEX authorization_assignments_mapping ON authorization_assignments(mapping_id) WHERE enabled;

-- No FK to live resource/owner rows: deleting or transferring a live resource
-- must never destroy historical evidence or let slug reuse inherit its access.
CREATE TABLE audit_resource_epochs (
 epoch_id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id text NOT NULL DEFAULT vmcp_tenant_id(),
 resource_kind text NOT NULL CHECK(resource_kind IN ('upstream','virtual_server')),
 resource_id uuid NOT NULL, project_id uuid, owner_team_id uuid,
 start_revision bigint NOT NULL CHECK(start_revision>0), end_revision bigint,
 CHECK(end_revision IS NULL OR end_revision>=start_revision)
);
CREATE UNIQUE INDEX audit_resource_epochs_open ON audit_resource_epochs(tenant_id,resource_kind,resource_id) WHERE end_revision IS NULL;
CREATE TABLE audit_access_contexts (
 event_id uuid PRIMARY KEY REFERENCES audit_events(event_id) ON DELETE CASCADE,
 tenant_id text NOT NULL DEFAULT vmcp_tenant_id(), context_version integer NOT NULL DEFAULT 1 CHECK(context_version=1),
 classification text NOT NULL CHECK(classification IN ('resource','tenant','legacy')),
 origin text NOT NULL, operation_config_revision bigint NOT NULL CHECK(operation_config_revision>=0),
 provenance_state text NOT NULL CHECK(provenance_state IN ('verified','legacy')),
 CHECK((classification='legacy')=(provenance_state='legacy'))
);
CREATE TABLE audit_access_domains (
 event_id uuid NOT NULL REFERENCES audit_access_contexts(event_id) ON DELETE CASCADE,
 domain_kind text NOT NULL CHECK(domain_kind IN ('consumer','provider','target')),
 resource_kind text NOT NULL CHECK(resource_kind IN ('upstream','virtual_server')),
 resource_id uuid NOT NULL, project_id uuid, owner_team_id uuid,
 epoch_id uuid NOT NULL REFERENCES audit_resource_epochs(epoch_id) ON DELETE RESTRICT,
 PRIMARY KEY(event_id,domain_kind,resource_kind,resource_id)
);
-- No event backfill: missing context is legacy, not evidence of current ownership.

-- +goose StatementBegin
CREATE FUNCTION bump_authorization_revision() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 UPDATE authorization_state SET revision=revision+1 WHERE singleton;
 RETURN NULL;
END $$;
CREATE TRIGGER authorization_roles_bump AFTER INSERT OR UPDATE OR DELETE ON authorization_roles FOR EACH STATEMENT EXECUTE FUNCTION bump_authorization_revision();
CREATE TRIGGER authorization_permissions_bump AFTER INSERT OR UPDATE OR DELETE ON authorization_permissions FOR EACH STATEMENT EXECUTE FUNCTION bump_authorization_revision();
CREATE TRIGGER authorization_role_permissions_bump AFTER INSERT OR UPDATE OR DELETE ON authorization_role_permissions FOR EACH STATEMENT EXECUTE FUNCTION bump_authorization_revision();
CREATE TRIGGER authorization_mappings_bump AFTER INSERT OR UPDATE OR DELETE ON authorization_mappings FOR EACH STATEMENT EXECUTE FUNCTION bump_authorization_revision();
CREATE TRIGGER authorization_assignments_bump AFTER INSERT OR UPDATE OR DELETE ON authorization_assignments FOR EACH STATEMENT EXECUTE FUNCTION bump_authorization_revision();
-- +goose StatementEnd

-- Permission catalog seeds follow. Defaults are explicit lists; future catalog
-- additions do not silently change custom roles.
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('platform.info.read',ARRAY['tenant'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('teams.read',ARRAY['tenant','team'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('teams.manage',ARRAY['tenant','team'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('projects.read',ARRAY['tenant','team','project','unmanaged'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('projects.manage',ARRAY['tenant','team','project','unmanaged'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('ownership.transfer',ARRAY['tenant','team','upstream','unmanaged','project','virtual_server'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('upstreams.read',ARRAY['tenant','team','upstream','unmanaged'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('upstreams.create',ARRAY['tenant','team','upstream','unmanaged'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('upstreams.update',ARRAY['tenant','team','upstream','unmanaged'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('upstreams.delete',ARRAY['tenant','team','upstream','unmanaged'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('upstreams.connection.update',ARRAY['tenant','team','upstream','unmanaged'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('upstreams.activation.update',ARRAY['tenant','team','upstream','unmanaged'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('upstreams.credentials.update',ARRAY['tenant','team','upstream','unmanaged'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('upstreams.tls.update',ARRAY['tenant','team','upstream','unmanaged'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('upstreams.identity_forwarding.update',ARRAY['tenant','team','upstream','unmanaged'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('upstreams.rest.manage',ARRAY['tenant','team','upstream','unmanaged'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('upstreams.catalog.read',ARRAY['tenant','team','upstream','unmanaged'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('upstreams.catalog.refresh',ARRAY['tenant','team','upstream','unmanaged'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('upstreams.test',ARRAY['tenant','team','upstream','unmanaged'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('upstreams.rest.preview',ARRAY['tenant','team','upstream','unmanaged'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('upstreams.rest.execute',ARRAY['tenant','team','upstream','unmanaged'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('upstreams.mock.read',ARRAY['tenant','team','upstream','unmanaged'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('upstreams.mock.sync',ARRAY['tenant','team','upstream','unmanaged'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('virtual_servers.read',ARRAY['tenant','team','project','virtual_server','unmanaged'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('virtual_servers.create',ARRAY['tenant','team','project','virtual_server','unmanaged'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('virtual_servers.update',ARRAY['tenant','team','project','virtual_server','unmanaged'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('virtual_servers.delete',ARRAY['tenant','team','project','virtual_server','unmanaged'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('virtual_servers.publication.update',ARRAY['tenant','team','project','virtual_server','unmanaged'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('virtual_servers.policy.update',ARRAY['tenant','team','project','virtual_server','unmanaged'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('virtual_servers.client_features.update',ARRAY['tenant','team','project','virtual_server','unmanaged'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('agent_keys.read',ARRAY['tenant','team','project','virtual_server','unmanaged'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('agent_keys.create',ARRAY['tenant','team','project','virtual_server','unmanaged'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('agent_keys.update',ARRAY['tenant','team','project','virtual_server','unmanaged'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('agent_keys.delete',ARRAY['tenant','team','project','virtual_server','unmanaged'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('agent_keys.access.update',ARRAY['tenant','team','project','virtual_server','unmanaged'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('agent_keys.rotate',ARRAY['tenant','team','project','virtual_server','unmanaged'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('agent_keys.revoke',ARRAY['tenant','team','project','virtual_server','unmanaged'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('agent_keys.constraints.update',ARRAY['tenant','team','project','virtual_server','unmanaged'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('agent_keys.audit_correlation.update',ARRAY['tenant','team','project','virtual_server','unmanaged'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('identity_bindings.read',ARRAY['tenant','team','project','virtual_server','unmanaged'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('identity_bindings.create',ARRAY['tenant','team','project','virtual_server','unmanaged'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('identity_bindings.update',ARRAY['tenant','team','project','virtual_server','unmanaged'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('identity_bindings.delete',ARRAY['tenant','team','project','virtual_server','unmanaged'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('identity_bindings.rules.update',ARRAY['tenant','team','project','virtual_server','unmanaged'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('identity_bindings.access.update',ARRAY['tenant','team','project','virtual_server','unmanaged'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('identity_bindings.constraints.update',ARRAY['tenant','team','project','virtual_server','unmanaged'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('identity_bindings.evaluate',ARRAY['tenant','team','project','virtual_server','unmanaged'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('control_tokens.read',ARRAY['tenant'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('control_tokens.create',ARRAY['tenant'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('control_tokens.update',ARRAY['tenant'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('control_tokens.delete',ARRAY['tenant'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('control_tokens.access.update',ARRAY['tenant'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('control_tokens.rotate',ARRAY['tenant'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('control_tokens.revoke',ARRAY['tenant'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('control_tokens.constraints.update',ARRAY['tenant'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('tool_catalog.read',ARRAY['tenant','team','project','virtual_server','unmanaged'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('tool_grants.read',ARRAY['tenant','team','project','upstream','virtual_server','unmanaged'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('tool_grants.request',ARRAY['tenant','team','project','upstream','virtual_server','unmanaged'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('tool_grants.approve',ARRAY['tenant','team','project','upstream','virtual_server','unmanaged'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('tool_grants.reject',ARRAY['tenant','team','project','upstream','virtual_server','unmanaged'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('tool_grants.revoke',ARRAY['tenant','team','project','upstream','virtual_server','unmanaged'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('connections.test_unsaved',ARRAY['tenant'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('rest.import',ARRAY['tenant'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('mock.provider.read',ARRAY['tenant'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('runtime_jwts.revoke',ARRAY['tenant'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('sessions.revoke',ARRAY['tenant'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('omni.status.read',ARRAY['tenant'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('omni.status.details.read',ARRAY['tenant'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('omni.preview',ARRAY['tenant'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('omni.reindex',ARRAY['tenant'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('config.export',ARRAY['tenant'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('config.validate',ARRAY['tenant'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('config.apply',ARRAY['tenant'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('config.prune',ARRAY['tenant'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('access.roles.read',ARRAY['tenant'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('access.roles.manage',ARRAY['tenant'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('access.assignments.read',ARRAY['tenant'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('access.assignments.manage',ARRAY['tenant'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('access.mappings.read',ARRAY['tenant'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('access.mappings.manage',ARRAY['tenant'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('access.evaluate',ARRAY['tenant'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('audit.events.read',ARRAY['tenant','team','project','upstream','virtual_server','unmanaged'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('audit.identities.read',ARRAY['tenant','team','project','upstream','virtual_server','unmanaged'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('audit.details.read',ARRAY['tenant','team','project','upstream','virtual_server','unmanaged'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('audit.settings.read',ARRAY['tenant','team','project','upstream','virtual_server','unmanaged'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('audit.redaction.local.update',ARRAY['tenant','team','project','upstream','virtual_server','unmanaged'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('audit.redaction.preview',ARRAY['tenant','team','project','upstream','virtual_server','unmanaged'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('audit.status.read',ARRAY['tenant'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('audit.durability.update',ARRAY['tenant'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('audit.retention.update',ARRAY['tenant'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('audit.capture.update',ARRAY['tenant'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('audit.redaction.global.update',ARRAY['tenant'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('audit.export_policy.update',ARRAY['tenant'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('audit.diagnostics.update',ARRAY['tenant'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('audit.diagnostics.denied.read',ARRAY['tenant'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('audit.legacy.events.read',ARRAY['tenant'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('audit.legacy.payloads.read',ARRAY['tenant'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('audit.legacy.payloads.unclassified.read',ARRAY['tenant'],false);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('audit.payloads.request.read',ARRAY['tenant','team','project','upstream','virtual_server','unmanaged'],true);
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES('audit.payloads.response.read',ARRAY['tenant','team','project','upstream','virtual_server','unmanaged'],true);
INSERT INTO authorization_roles(slug,name,builtin) VALUES('platform-admin','Platform Admin',true),('observer','Observer',true);
INSERT INTO authorization_role_permissions(role_id,permission_id) SELECT r.id,p.id FROM authorization_roles r CROSS JOIN authorization_permissions p WHERE r.slug='platform-admin';
INSERT INTO authorization_role_permissions(role_id,permission_id) SELECT r.id,p.id FROM authorization_roles r CROSS JOIN authorization_permissions p WHERE r.slug='observer' AND p.id IN ('platform.info.read','upstreams.read','upstreams.catalog.read','virtual_servers.read','projects.read','teams.read');

-- Task-specific default templates. No assignments are created by this migration.
INSERT INTO authorization_roles(slug,name,builtin) VALUES
 ('upstream-maintainer','Upstream Maintainer',true),
 ('virtual-server-editor','Virtual Server Editor',true),
 ('credential-manager','Credential Manager',true),
 ('access-approver','Access Approver',true),
 ('auditor','Auditor',true),
 ('automation-deployer','Automation Deployer',true);
INSERT INTO authorization_role_permissions(role_id,permission_id)
SELECT r.id,p.id FROM authorization_roles r CROSS JOIN authorization_permissions p WHERE
 (r.slug='upstream-maintainer' AND p.id IN ('upstreams.read','upstreams.update','upstreams.connection.update','upstreams.test','upstreams.catalog.read','upstreams.catalog.refresh')) OR
 (r.slug='virtual-server-editor' AND p.id IN ('virtual_servers.read','virtual_servers.create','virtual_servers.update','tool_catalog.read','tool_grants.read','tool_grants.request')) OR
 (r.slug='credential-manager' AND p.id IN ('agent_keys.read','agent_keys.create','agent_keys.update','agent_keys.access.update','agent_keys.constraints.update','agent_keys.rotate','agent_keys.revoke','identity_bindings.read','identity_bindings.create','identity_bindings.update','identity_bindings.rules.update','identity_bindings.access.update','identity_bindings.constraints.update')) OR
 (r.slug='access-approver' AND p.id IN ('tool_grants.read','tool_grants.approve','tool_grants.reject','tool_grants.revoke')) OR
 (r.slug='auditor' AND p.id='audit.events.read') OR
 (r.slug='automation-deployer' AND p.id IN ('config.export','config.validate','config.apply','upstreams.read','upstreams.create','upstreams.update','upstreams.connection.update','virtual_servers.read','virtual_servers.create','virtual_servers.update'));

-- Immutable context snapshots cannot be rewritten as current ownership changes.
-- +goose StatementBegin
CREATE FUNCTION validate_audit_context() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE recorded_tenant text;
BEGIN
 SELECT tenant_id INTO recorded_tenant FROM audit_events WHERE event_id=NEW.event_id;
 IF recorded_tenant IS NULL OR recorded_tenant<>NEW.tenant_id THEN
  RAISE EXCEPTION 'audit context tenant does not match event';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER audit_access_contexts_validate BEFORE INSERT ON audit_access_contexts FOR EACH ROW EXECUTE FUNCTION validate_audit_context();
CREATE FUNCTION reject_audit_context_update() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'audit access provenance is immutable'; END $$;
CREATE TRIGGER audit_access_contexts_immutable BEFORE UPDATE ON audit_access_contexts FOR EACH ROW EXECUTE FUNCTION reject_audit_context_update();
CREATE TRIGGER audit_access_domains_immutable BEFORE UPDATE ON audit_access_domains FOR EACH ROW EXECUTE FUNCTION reject_audit_context_update();
CREATE FUNCTION validate_audit_domain() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE e audit_resource_epochs; c audit_access_contexts;
BEGIN
 SELECT * INTO e FROM audit_resource_epochs WHERE epoch_id=NEW.epoch_id;
 SELECT * INTO c FROM audit_access_contexts WHERE event_id=NEW.event_id;
 IF e.epoch_id IS NULL OR c.event_id IS NULL OR c.classification<>'resource' OR
    e.tenant_id<>c.tenant_id OR e.resource_kind<>NEW.resource_kind OR e.resource_id<>NEW.resource_id OR
    e.project_id IS DISTINCT FROM NEW.project_id OR e.owner_team_id IS DISTINCT FROM NEW.owner_team_id THEN
  RAISE EXCEPTION 'audit domain does not match its recorded epoch and tenant';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER audit_access_domains_validate BEFORE INSERT ON audit_access_domains FOR EACH ROW EXECUTE FUNCTION validate_audit_domain();
CREATE FUNCTION protect_audit_epoch() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF ROW(NEW.epoch_id,NEW.tenant_id,NEW.resource_kind,NEW.resource_id,NEW.project_id,NEW.owner_team_id,NEW.start_revision)
    IS DISTINCT FROM ROW(OLD.epoch_id,OLD.tenant_id,OLD.resource_kind,OLD.resource_id,OLD.project_id,OLD.owner_team_id,OLD.start_revision)
    OR OLD.end_revision IS NOT NULL OR NEW.end_revision IS NULL THEN
  RAISE EXCEPTION 'audit epoch may only be closed once';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER audit_resource_epochs_protect BEFORE UPDATE ON audit_resource_epochs FOR EACH ROW EXECUTE FUNCTION protect_audit_epoch();
-- +goose StatementEnd

-- +goose Down
DROP TABLE audit_access_domains;
DROP TABLE audit_access_contexts;
DROP TABLE audit_resource_epochs;
DROP FUNCTION reject_audit_context_update();
DROP FUNCTION validate_audit_context();
DROP FUNCTION validate_audit_domain();
DROP FUNCTION protect_audit_epoch();
DROP TABLE authorization_assignments;
DROP TABLE authorization_mappings;
DROP TABLE authorization_role_permissions;
DROP TABLE authorization_roles;
DROP TABLE authorization_permissions;
DROP FUNCTION bump_authorization_revision();
DROP TABLE authorization_state;
