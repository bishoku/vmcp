-- +goose Up
CREATE TABLE tenant_runtime_identity (
 singleton boolean PRIMARY KEY DEFAULT true CHECK(singleton),
 tenant_id text NOT NULL UNIQUE
);
INSERT INTO tenant_runtime_identity(tenant_id) VALUES('default');
-- +goose StatementBegin
CREATE FUNCTION vmcp_tenant_id() RETURNS text LANGUAGE sql STABLE AS $$ SELECT tenant_id FROM tenant_runtime_identity WHERE singleton $$;
-- +goose StatementEnd
ALTER TABLE audit_events ADD COLUMN tenant_id text NOT NULL DEFAULT vmcp_tenant_id();
-- +goose Down
ALTER TABLE audit_events DROP COLUMN tenant_id;
DROP FUNCTION vmcp_tenant_id();
DROP TABLE tenant_runtime_identity;
