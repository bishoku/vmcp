-- +goose Up
-- Per-upstream TLS: custom CA bundle (PEM), SNI/verification name override and optional mTLS client certificate.
ALTER TABLE upstreams
    ADD COLUMN ca_pem text NOT NULL DEFAULT '',
    ADD COLUMN tls_server_name text NOT NULL DEFAULT '',
    ADD COLUMN client_cert_pem text NOT NULL DEFAULT '',
    ADD COLUMN encrypted_client_key bytea;

-- +goose Down
ALTER TABLE upstreams
    DROP COLUMN encrypted_client_key,
    DROP COLUMN client_cert_pem,
    DROP COLUMN tls_server_name,
    DROP COLUMN ca_pem;
