-- +goose Up
-- Legacy sessions intentionally require reconnect: their subject cannot be inferred safely.
ALTER TABLE sessions
 ADD COLUMN owner_mode text NOT NULL DEFAULT 'legacy',
 ADD COLUMN credential_id text NOT NULL DEFAULT '',
 ADD COLUMN owner_issuer text NOT NULL DEFAULT '',
 ADD COLUMN owner_subject text NOT NULL DEFAULT '',
 ADD COLUMN terminated_at timestamptz;
CREATE INDEX sessions_terminated_at_idx ON sessions (terminated_at) WHERE terminated_at IS NOT NULL;

-- +goose Down
DROP INDEX sessions_terminated_at_idx;
ALTER TABLE sessions DROP COLUMN owner_mode, DROP COLUMN credential_id,
 DROP COLUMN owner_issuer, DROP COLUMN owner_subject, DROP COLUMN terminated_at;
