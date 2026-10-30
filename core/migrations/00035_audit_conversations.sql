-- +goose Up
ALTER TABLE agent_keys ADD COLUMN audit_correlation jsonb NOT NULL DEFAULT '{}';
ALTER TABLE audit_events
 ADD COLUMN conversation_id text NOT NULL DEFAULT '',
 ADD COLUMN root_conversation_id text NOT NULL DEFAULT '',
 ADD COLUMN turn_id text NOT NULL DEFAULT '',
 ADD COLUMN conversation_group_id text NOT NULL DEFAULT '',
 ADD COLUMN correlation_status text NOT NULL DEFAULT '';
CREATE INDEX audit_conversation_group_idx ON audit_events(tenant_id,conversation_group_id,id DESC) WHERE conversation_group_id<>'';
CREATE INDEX audit_conversation_id_idx ON audit_events(tenant_id,conversation_id,id DESC) WHERE conversation_id<>'';
CREATE INDEX audit_root_conversation_id_idx ON audit_events(tenant_id,root_conversation_id,id DESC) WHERE root_conversation_id<>'';
-- +goose Down
DROP INDEX audit_root_conversation_id_idx;
DROP INDEX audit_conversation_id_idx;
DROP INDEX audit_conversation_group_idx;
ALTER TABLE audit_events DROP COLUMN correlation_status, DROP COLUMN conversation_group_id, DROP COLUMN turn_id, DROP COLUMN root_conversation_id, DROP COLUMN conversation_id;
ALTER TABLE agent_keys DROP COLUMN audit_correlation;
