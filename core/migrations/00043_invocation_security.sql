-- +goose Up
CREATE TABLE invocation_security (
 singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
 version bigint NOT NULL DEFAULT 1,
 ciphertext bytea
);
INSERT INTO invocation_security(singleton) VALUES(true);
CREATE TRIGGER invocation_security_revision AFTER UPDATE ON invocation_security FOR EACH STATEMENT EXECUTE FUNCTION bump_config_revision();
-- Consumed operation IDs cannot be used for a second dispatch, including across replicas.
CREATE TABLE invocation_dispatches (
 operation_id text PRIMARY KEY,
 binding_hash text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now()
);
-- +goose Down
DROP TABLE invocation_dispatches;
DROP TABLE invocation_security;
