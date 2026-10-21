-- +goose Up
CREATE TABLE audit_settings (
 singleton boolean PRIMARY KEY DEFAULT true CHECK(singleton),
 version bigint NOT NULL DEFAULT 1,
 settings jsonb NOT NULL DEFAULT '{}'
);
INSERT INTO audit_settings(singleton) VALUES(true);
CREATE TRIGGER audit_settings_bump AFTER INSERT OR UPDATE OR DELETE ON audit_settings
 FOR EACH STATEMENT EXECUTE FUNCTION bump_config_revision();
ALTER TABLE audit_events
 ADD COLUMN audit_mode text NOT NULL DEFAULT 'legacy',
 ADD COLUMN policy_revision bigint NOT NULL DEFAULT 0,
 ADD COLUMN scope text NOT NULL DEFAULT '',
 ADD COLUMN boundary text NOT NULL DEFAULT '',
 ADD COLUMN direction text NOT NULL DEFAULT '',
 ADD COLUMN parent_operation_id uuid,
 ADD COLUMN payload_state text NOT NULL DEFAULT 'disabled',
 ADD COLUMN payload_original_bytes integer NOT NULL DEFAULT 0,
 ADD COLUMN payload_stored_bytes integer NOT NULL DEFAULT 0,
 ADD COLUMN payload_redactions integer NOT NULL DEFAULT 0;
CREATE INDEX audit_events_parent_operation_idx ON audit_events(parent_operation_id) WHERE parent_operation_id IS NOT NULL;
CREATE TABLE audit_payloads (
 event_id uuid PRIMARY KEY REFERENCES audit_events(event_id) ON DELETE CASCADE,
 tenant_id text NOT NULL DEFAULT vmcp_tenant_id(),
 ciphertext bytea NOT NULL,
 expires_at timestamptz NOT NULL
);
CREATE INDEX audit_payloads_expiry_idx ON audit_payloads(expires_at);
ALTER TABLE audit_export_state ADD COLUMN include_payload boolean NOT NULL DEFAULT false;
-- The exact bytes of a delivery survive retries and payload expiry. Encrypted at rest.
ALTER TABLE audit_deliveries ADD COLUMN envelope bytea;

-- +goose Down
ALTER TABLE audit_deliveries DROP COLUMN envelope;
ALTER TABLE audit_export_state DROP COLUMN include_payload;
DROP TABLE audit_payloads;
DROP INDEX audit_events_parent_operation_idx;
ALTER TABLE audit_events DROP COLUMN audit_mode, DROP COLUMN policy_revision,
 DROP COLUMN scope, DROP COLUMN boundary, DROP COLUMN direction, DROP COLUMN parent_operation_id,
 DROP COLUMN payload_state, DROP COLUMN payload_original_bytes, DROP COLUMN payload_stored_bytes, DROP COLUMN payload_redactions;
DROP TABLE audit_settings;
