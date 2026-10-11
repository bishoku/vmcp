-- +goose Up
CREATE TABLE browser_sessions (
 id text PRIMARY KEY,
 subject text NOT NULL,
 expires_at timestamptz NOT NULL,
 revoked_at timestamptz
);
CREATE INDEX browser_sessions_subject ON browser_sessions(subject);
CREATE INDEX browser_sessions_expiry ON browser_sessions(expires_at);
CREATE TABLE revoked_jwts (
 token_hash bytea PRIMARY KEY CHECK(octet_length(token_hash)=32),
 expires_at timestamptz NOT NULL
);
CREATE INDEX revoked_jwts_expiry ON revoked_jwts(expires_at);
-- +goose Down
DROP TABLE revoked_jwts;
DROP TABLE browser_sessions;
