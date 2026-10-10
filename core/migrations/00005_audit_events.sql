-- +goose Up
-- Written by the data plane at high volume, so deliberately no revision trigger
-- and no foreign keys: events must outlive the keys and servers they mention.
CREATE TABLE audit_events (
    id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    ts              timestamptz NOT NULL DEFAULT now(),
    virtual_server  text NOT NULL,
    agent_key       text,
    client_ip       text,
    method          text NOT NULL,
    tool            text,
    upstream        text,
    status          text NOT NULL CHECK (status IN ('ok', 'tool_error', 'error', 'denied')),
    reason          text,
    duration_ms     integer NOT NULL DEFAULT 0,
    redactions      integer NOT NULL DEFAULT 0
);
CREATE INDEX audit_events_ts_idx ON audit_events (ts DESC);
CREATE INDEX audit_events_server_idx ON audit_events (virtual_server, id DESC);
CREATE INDEX audit_events_key_idx ON audit_events (agent_key, id DESC);

-- +goose Down
DROP TABLE audit_events;
