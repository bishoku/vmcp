-- +goose Up
-- Explicit builtin template dependencies; custom copies and assignments are preserved.
WITH dependencies(slug,permission) AS (VALUES
 ('virtual-server-editor','projects.read'),
 ('credential-manager','virtual_servers.read'),
 ('automation-deployer','upstreams.catalog.read'),
 ('automation-deployer','upstreams.catalog.refresh'),
 ('automation-deployer','upstreams.activation.update'),
 ('automation-deployer','upstreams.credentials.update'),
 ('automation-deployer','upstreams.tls.update'),
 ('automation-deployer','upstreams.identity_forwarding.update'),
 ('automation-deployer','upstreams.rest.manage'),
 ('automation-deployer','virtual_servers.publication.update'),
 ('automation-deployer','virtual_servers.policy.update'),
 ('automation-deployer','virtual_servers.client_features.update'),
 ('automation-deployer','identity_bindings.read'),
 ('automation-deployer','teams.read'),
 ('automation-deployer','projects.read'),
 ('automation-deployer','tool_grants.read'),
 ('automation-deployer','audit.settings.read'))
INSERT INTO authorization_role_permissions(role_id,permission_id)
SELECT r.id,d.permission FROM dependencies d JOIN authorization_roles r ON r.slug=d.slug AND r.builtin
ON CONFLICT DO NOTHING;
UPDATE authorization_roles SET version=version+1 WHERE builtin AND slug IN ('virtual-server-editor','credential-manager','automation-deployer');

-- +goose Down
WITH dependencies(slug,permission) AS (VALUES
 ('virtual-server-editor','projects.read'),
 ('credential-manager','virtual_servers.read'),
 ('automation-deployer','upstreams.catalog.read'),
 ('automation-deployer','upstreams.catalog.refresh'),
 ('automation-deployer','upstreams.activation.update'),
 ('automation-deployer','upstreams.credentials.update'),
 ('automation-deployer','upstreams.tls.update'),
 ('automation-deployer','upstreams.identity_forwarding.update'),
 ('automation-deployer','upstreams.rest.manage'),
 ('automation-deployer','virtual_servers.publication.update'),
 ('automation-deployer','virtual_servers.policy.update'),
 ('automation-deployer','virtual_servers.client_features.update'),
 ('automation-deployer','identity_bindings.read'),
 ('automation-deployer','teams.read'),
 ('automation-deployer','projects.read'),
 ('automation-deployer','tool_grants.read'),
 ('automation-deployer','audit.settings.read'))
DELETE FROM authorization_role_permissions p USING authorization_roles r,dependencies d
WHERE p.role_id=r.id AND r.builtin AND r.slug=d.slug AND p.permission_id=d.permission;
UPDATE authorization_roles SET version=greatest(1,version-1) WHERE builtin AND slug IN ('virtual-server-editor','credential-manager','automation-deployer');
