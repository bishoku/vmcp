-- +goose Up
-- A payload-only rollout must not activate unfinished authorization for other APIs.
ALTER TABLE authorization_state ADD COLUMN audit_payload_mode text NOT NULL DEFAULT 'legacy'
 CHECK(audit_payload_mode IN ('legacy','shadow','enforced'));
-- +goose StatementBegin
CREATE FUNCTION authorization_mode_revision() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.mode IS DISTINCT FROM OLD.mode OR NEW.audit_payload_mode IS DISTINCT FROM OLD.audit_payload_mode THEN
  NEW.revision=OLD.revision+1;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER authorization_mode_revision BEFORE UPDATE ON authorization_state FOR EACH ROW EXECUTE FUNCTION authorization_mode_revision();
-- +goose StatementEnd
-- +goose Down
DROP TRIGGER authorization_mode_revision ON authorization_state;
DROP FUNCTION authorization_mode_revision();
ALTER TABLE authorization_state DROP COLUMN audit_payload_mode;
