-- +goose Up
-- Existing sessions must complete the initialized notification after upgrading.
ALTER TABLE sessions ADD COLUMN initialized boolean NOT NULL DEFAULT false;
ALTER TABLE sessions ADD COLUMN client_capabilities jsonb NOT NULL DEFAULT '{}'::jsonb;

-- +goose Down
ALTER TABLE sessions DROP COLUMN client_capabilities;
ALTER TABLE sessions DROP COLUMN initialized;
