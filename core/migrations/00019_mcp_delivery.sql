-- +goose Up
CREATE TABLE mcp_operations (
 id uuid PRIMARY KEY,
 session_id text NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
 client_request_id text NOT NULL CHECK(octet_length(client_request_id)<=1024),
 principal_hash bytea NOT NULL,
 virtual_server text NOT NULL,
 fence uuid NOT NULL,
 lease_until timestamptz NOT NULL,
 cancelled boolean NOT NULL DEFAULT false,
 state text NOT NULL DEFAULT 'running' CHECK(state IN ('running','done','abandoned')),
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX mcp_operation_request ON mcp_operations(session_id,client_request_id) WHERE state='running';
CREATE INDEX mcp_operation_expiry ON mcp_operations(lease_until) WHERE state='running';
CREATE INDEX mcp_operation_principal ON mcp_operations(principal_hash) WHERE state='running';
CREATE TABLE mcp_streams (
 id uuid PRIMARY KEY,
 session_id text NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
 operation_id uuid REFERENCES mcp_operations(id) ON DELETE CASCADE,
 owner uuid,
 lease_until timestamptz,
 closed boolean NOT NULL DEFAULT false,
 last_revision bigint NOT NULL DEFAULT 0,
 expires_at timestamptz NOT NULL DEFAULT now()+interval '15 minutes'
);
CREATE UNIQUE INDEX mcp_get_stream ON mcp_streams(session_id) WHERE operation_id IS NULL AND NOT closed;
CREATE INDEX mcp_stream_expiry ON mcp_streams(expires_at);
CREATE TABLE mcp_events (
 seq bigserial PRIMARY KEY,
 id uuid UNIQUE NOT NULL,
 stream_id uuid NOT NULL REFERENCES mcp_streams(id) ON DELETE CASCADE,
 payload bytea NOT NULL CHECK(octet_length(payload)<=10486000),
 terminal boolean NOT NULL DEFAULT false,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX mcp_event_replay ON mcp_events(stream_id,seq);
CREATE TABLE mcp_pending_requests (
 id uuid PRIMARY KEY,
 operation_id uuid NOT NULL REFERENCES mcp_operations(id) ON DELETE CASCADE,
 session_id text NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
 response bytea,
 expires_at timestamptz NOT NULL
);
CREATE TABLE mcp_discovery_requests (
 upstream_id uuid PRIMARY KEY REFERENCES upstreams(id) ON DELETE CASCADE,
 requested_at timestamptz NOT NULL DEFAULT now(),
 lease_owner uuid,
 lease_until timestamptz
);

-- +goose Down
DROP TABLE mcp_discovery_requests;
DROP TABLE mcp_pending_requests;
DROP TABLE mcp_events;
DROP TABLE mcp_streams;
DROP TABLE mcp_operations;
