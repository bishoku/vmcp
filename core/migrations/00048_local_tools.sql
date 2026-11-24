-- +goose Up
ALTER TABLE upstreams DROP CONSTRAINT upstreams_kind_check,
 ADD CONSTRAINT upstreams_kind_check CHECK (kind IN ('mcp','rest','local')),
 ADD COLUMN local_config jsonb;

-- +goose Down
ALTER TABLE upstreams DROP CONSTRAINT upstreams_kind_check,
 ADD CONSTRAINT upstreams_kind_check CHECK (kind IN ('mcp','rest')),
 DROP COLUMN local_config;
