-- +goose Up
-- Extra URLs of the same upstream, tried in order when the primary does not answer.
ALTER TABLE upstreams ADD COLUMN fallback_urls text[] NOT NULL DEFAULT '{}';

-- What each data-plane node last saw when probing an upstream. Operational state, not
-- configuration: no revision trigger. Rows older than a couple of minutes are ignored (dead node).
CREATE TABLE upstream_health (
    upstream_id uuid NOT NULL REFERENCES upstreams(id) ON DELETE CASCADE,
    node        text NOT NULL,
    state       text NOT NULL CHECK (state IN ('healthy', 'unhealthy')),
    breaker     text NOT NULL DEFAULT 'closed',
    url         text NOT NULL DEFAULT '',
    latency_ms  integer NOT NULL DEFAULT 0,
    error       text,
    checked_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (upstream_id, node)
);

-- +goose Down
DROP TABLE upstream_health;
ALTER TABLE upstreams DROP COLUMN fallback_urls;
