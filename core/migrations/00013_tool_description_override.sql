-- +goose Up
-- Per-selection description override, applied only in tools/list of that virtual server.
-- description_basis records the upstream text at the time of writing, to detect drift.
ALTER TABLE virtual_server_tools ADD COLUMN description text, ADD COLUMN description_basis text;

-- +goose Down
ALTER TABLE virtual_server_tools DROP COLUMN description_basis, DROP COLUMN description;
