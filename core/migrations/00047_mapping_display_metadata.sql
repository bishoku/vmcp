-- +goose Up
ALTER TABLE authorization_mappings
 ADD COLUMN name text NOT NULL DEFAULT '' CHECK (octet_length(name) <= 200),
 ADD COLUMN description text NOT NULL DEFAULT '' CHECK (octet_length(description) <= 4000);

-- +goose Down
ALTER TABLE authorization_mappings DROP COLUMN description, DROP COLUMN name;
