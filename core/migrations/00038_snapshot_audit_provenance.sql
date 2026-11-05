-- +goose Up
-- Epochs describe ownership from this migration forward, not historical events.
INSERT INTO audit_resource_epochs(resource_kind,resource_id,owner_team_id,start_revision)
 SELECT 'upstream',id,owner_team_id,(SELECT revision FROM config_revision) FROM upstreams ON CONFLICT DO NOTHING;
INSERT INTO audit_resource_epochs(resource_kind,resource_id,project_id,owner_team_id,start_revision)
 SELECT 'virtual_server',v.id,v.project_id,p.owner_team_id,(SELECT revision FROM config_revision)
 FROM virtual_servers v JOIN projects p ON p.id=v.project_id ON CONFLICT DO NOTHING;

-- Deferred triggers run after revision bumps and see the final configuration in
-- the transaction. They cover SQL/GitOps writers, not only HTTP ownership APIs.
-- +goose StatementBegin
CREATE FUNCTION sync_audit_resource_epoch(k text,rid uuid,pid uuid,tid uuid,removed boolean) RETURNS void LANGUAGE plpgsql AS $$
DECLARE current_epoch audit_resource_epochs; rev bigint; tenant text;
BEGIN
 SELECT revision INTO rev FROM config_revision FOR UPDATE;
 tenant=vmcp_tenant_id();
 SELECT * INTO current_epoch FROM audit_resource_epochs WHERE tenant_id=tenant AND resource_kind=k AND resource_id=rid AND end_revision IS NULL FOR UPDATE;
 IF current_epoch.epoch_id IS NOT NULL THEN
  IF NOT removed AND current_epoch.project_id IS NOT DISTINCT FROM pid AND current_epoch.owner_team_id IS NOT DISTINCT FROM tid THEN RETURN; END IF;
  UPDATE audit_resource_epochs SET end_revision=rev WHERE epoch_id=current_epoch.epoch_id;
  -- Historical epoch-pinned audit assignments survive; live resource grants do not.
  UPDATE authorization_assignments SET enabled=false,version=version+1
   WHERE scope_kind=k AND scope_id=rid AND epoch_id IS NULL AND enabled;
 END IF;
 IF NOT removed THEN
  INSERT INTO audit_resource_epochs(tenant_id,resource_kind,resource_id,project_id,owner_team_id,start_revision) VALUES(tenant,k,rid,pid,tid,rev);
 END IF;
END $$;
CREATE FUNCTION maintain_audit_resource_epoch() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE rid uuid; pid uuid; tid uuid;
BEGIN
 rid=CASE WHEN TG_OP='DELETE' THEN OLD.id ELSE NEW.id END;
 IF TG_TABLE_NAME='upstreams' THEN
  -- At deferred execution the resource might have been changed/deleted again.
  SELECT owner_team_id INTO tid FROM upstreams WHERE id=rid;
  PERFORM sync_audit_resource_epoch('upstream',rid,NULL,tid,NOT FOUND);
 ELSE
  SELECT v.project_id,p.owner_team_id INTO pid,tid FROM virtual_servers v JOIN projects p ON p.id=v.project_id WHERE v.id=rid;
  PERFORM sync_audit_resource_epoch('virtual_server',rid,pid,tid,NOT FOUND);
 END IF;
 RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER upstream_audit_epoch AFTER INSERT OR UPDATE OR DELETE ON upstreams DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION maintain_audit_resource_epoch();
CREATE CONSTRAINT TRIGGER virtual_audit_epoch AFTER INSERT OR UPDATE OR DELETE ON virtual_servers DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION maintain_audit_resource_epoch();
CREATE FUNCTION maintain_project_audit_epochs() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE v record;
BEGIN
 IF NEW.owner_team_id IS NOT DISTINCT FROM OLD.owner_team_id THEN RETURN NULL; END IF;
 FOR v IN SELECT vs.id,vs.project_id,p.owner_team_id FROM virtual_servers vs JOIN projects p ON p.id=vs.project_id WHERE vs.project_id=NEW.id LOOP
  PERFORM sync_audit_resource_epoch('virtual_server',v.id,v.project_id,v.owner_team_id,false);
 END LOOP;
 RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER project_audit_epochs AFTER UPDATE ON projects DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION maintain_project_audit_epochs();
-- +goose StatementEnd

-- Re-upgrade reconciles the baseline without changing any historical event.
SELECT sync_audit_resource_epoch('upstream',id,NULL,owner_team_id,false) FROM upstreams;
SELECT sync_audit_resource_epoch('virtual_server',v.id,v.project_id,p.owner_team_id,false) FROM virtual_servers v JOIN projects p ON p.id=v.project_id;
SELECT sync_audit_resource_epoch(resource_kind,resource_id,NULL,NULL,true) FROM audit_resource_epochs e WHERE end_revision IS NULL AND
 ((resource_kind='upstream' AND NOT EXISTS(SELECT 1 FROM upstreams u WHERE u.id=e.resource_id)) OR
  (resource_kind='virtual_server' AND NOT EXISTS(SELECT 1 FROM virtual_servers v WHERE v.id=e.resource_id)));

-- +goose Down
DROP TRIGGER project_audit_epochs ON projects;
DROP TRIGGER virtual_audit_epoch ON virtual_servers;
DROP TRIGGER upstream_audit_epoch ON upstreams;
DROP FUNCTION maintain_project_audit_epochs();
DROP FUNCTION maintain_audit_resource_epoch();
DROP FUNCTION sync_audit_resource_epoch(text,uuid,uuid,uuid,boolean);
-- Keep epoch evidence on rollback: retained event contexts can still refer to it.
