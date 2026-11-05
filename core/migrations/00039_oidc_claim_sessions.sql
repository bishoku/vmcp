-- +goose Up
-- Existing browser sessions remain version 0 and require reauthentication on
-- upgraded control-plane nodes. Never infer an issuer or claim provenance.
ALTER TABLE browser_sessions ADD COLUMN issuer text NOT NULL DEFAULT '';
ALTER TABLE browser_sessions ADD COLUMN model_version integer NOT NULL DEFAULT 0 CHECK(model_version IN (0,1));
ALTER TABLE browser_sessions ADD COLUMN session_ciphertext bytea;
ALTER TABLE browser_sessions ADD CONSTRAINT browser_session_model CHECK
 ((model_version=0 AND session_ciphertext IS NULL) OR
  (model_version=1 AND issuer<>'' AND session_ciphertext IS NOT NULL));
CREATE INDEX browser_sessions_issuer_subject ON browser_sessions(issuer,subject);

-- +goose Down
-- Do not leave a new reference cookie usable against a downgraded server.
DELETE FROM browser_sessions WHERE model_version=1;
DROP INDEX browser_sessions_issuer_subject;
ALTER TABLE browser_sessions DROP CONSTRAINT browser_session_model;
ALTER TABLE browser_sessions DROP COLUMN session_ciphertext;
ALTER TABLE browser_sessions DROP COLUMN model_version;
ALTER TABLE browser_sessions DROP COLUMN issuer;
