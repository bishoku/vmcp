-- +goose Up
ALTER TABLE upstreams DROP CONSTRAINT upstreams_transport_check;
ALTER TABLE upstreams ADD CONSTRAINT upstreams_transport_check CHECK(transport IN ('streamable_http','sse','stdio_worker'));
-- +goose Down
-- Refuse a destructive downgrade while worker configurations exist.
ALTER TABLE upstreams DROP CONSTRAINT upstreams_transport_check;
ALTER TABLE upstreams ADD CONSTRAINT upstreams_transport_check CHECK(transport IN ('streamable_http','sse'));
