-- +goose Up
ALTER TABLE upstreams ADD COLUMN invocation_security_revision bigint NOT NULL DEFAULT 0;
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES
 ('upstreams.invocation_security.read',ARRAY['tenant','team','upstream','unmanaged'],false),
 ('upstreams.invocation_security.manage',ARRAY['tenant','team','upstream','unmanaged'],false);
INSERT INTO authorization_role_permissions(role_id,permission_id)
 SELECT r.id,p.id FROM authorization_roles r CROSS JOIN authorization_permissions p
 WHERE r.builtin AND r.slug='platform-operator' AND p.id LIKE 'upstreams.invocation_security.%';

-- +goose Down
DELETE FROM authorization_role_permissions WHERE permission_id LIKE 'upstreams.invocation_security.%';
DELETE FROM authorization_permissions WHERE id LIKE 'upstreams.invocation_security.%';
ALTER TABLE upstreams DROP COLUMN invocation_security_revision;
