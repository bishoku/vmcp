-- +goose Up
-- Client sessions of the data plane, so any node can resume a session another node opened.
-- Only identifying facts are stored; upstream sessions are re-established lazily by the node that
-- serves the next request. No revision trigger: not configuration.
CREATE TABLE sessions (
    id               text PRIMARY KEY,
    virtual_server   text NOT NULL,
    key_slug         text NOT NULL DEFAULT '',
    protocol_version text NOT NULL,
    created_at       timestamptz NOT NULL DEFAULT now(),
    last_seen_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX sessions_last_seen_idx ON sessions (last_seen_at);

-- +goose Down
DROP TABLE sessions;
