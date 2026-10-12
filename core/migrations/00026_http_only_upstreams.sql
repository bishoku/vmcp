-- +goose Up
-- Existing unsupported configurations must be removed before upgrading.
-- Validate in place: never delete upstreams or their virtual-server bindings.
ALTER TABLE upstreams DROP CONSTRAINT upstreams_transport_check;
ALTER TABLE upstreams ADD CONSTRAINT upstreams_transport_check CHECK (transport IN ('streamable_http', 'sse'));
-- +goose Down
-- HTTP-only transport validation remains enforced on rollback.
SELECT 1;
