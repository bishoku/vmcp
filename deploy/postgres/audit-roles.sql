-- Run as the database owner after migrations. Provision LOGIN roles and TLS
-- credentials separately; never grant application roles membership in owner.
CREATE ROLE vmcp_audit_query NOLOGIN;
CREATE ROLE vmcp_audit_export NOLOGIN;
CREATE ROLE vmcp_audit_retention NOLOGIN;
GRANT USAGE ON SCHEMA public TO vmcp_audit_query, vmcp_audit_export, vmcp_audit_retention;
-- Export startup verifies the dedicated database identity. INSERT defaults use
-- vmcp_tenant_id(), which runs with the caller's identity-table privileges.
GRANT SELECT ON tenant_runtime_identity TO vmcp_audit_export;
GRANT SELECT ON tenants TO vmcp_audit_export;
GRANT SELECT ON audit_events TO vmcp_audit_query, vmcp_audit_export, vmcp_audit_retention;
GRANT SELECT,INSERT,UPDATE ON audit_export_state,audit_deliveries TO vmcp_audit_export;
GRANT SELECT ON audit_export_state,audit_deliveries,audit_legal_holds TO vmcp_audit_retention;
GRANT DELETE ON audit_events TO vmcp_audit_retention;
-- The gateway's separately provisioned application LOGIN role needs INSERT on
-- audit_events plus USAGE on audit_events_id_seq, not UPDATE/DELETE on audit_events.
-- Retention SELECT FOR UPDATE also needs UPDATE on at least one audit_events
-- column in PostgreSQL; isolate this privilege to the retention role.
GRANT UPDATE(id) ON audit_events TO vmcp_audit_retention;
-- A privileged evidence operator owns legal-hold creation/removal and consumer
-- activation/deactivation. Those powers are deliberately absent above.

-- Payload tables remain separate from metadata query access.
GRANT SELECT ON audit_settings TO vmcp_audit_export;
GRANT SELECT ON audit_payloads TO vmcp_audit_export, vmcp_audit_retention;
GRANT DELETE ON audit_payloads TO vmcp_audit_retention;
GRANT UPDATE(event_id) ON audit_payloads TO vmcp_audit_retention;
