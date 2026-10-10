-- +goose Up
-- Gateway -> upstream OAuth 2.0 client_credentials. The client secret is encrypted like upstream headers.
ALTER TABLE upstreams
    ADD COLUMN oauth_token_url text NOT NULL DEFAULT '',
    ADD COLUMN oauth_client_id text NOT NULL DEFAULT '',
    ADD COLUMN oauth_scopes text[] NOT NULL DEFAULT '{}',
    ADD COLUMN oauth_audience text NOT NULL DEFAULT '',
    ADD COLUMN encrypted_oauth_secret bytea;

-- +goose Down
ALTER TABLE upstreams
    DROP COLUMN encrypted_oauth_secret,
    DROP COLUMN oauth_audience,
    DROP COLUMN oauth_scopes,
    DROP COLUMN oauth_client_id,
    DROP COLUMN oauth_token_url;
