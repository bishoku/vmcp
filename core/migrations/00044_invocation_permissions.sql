-- +goose Up
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES
 ('invocation_security.read',ARRAY['tenant'],false),
 ('invocation_security.manage',ARRAY['tenant'],false);
INSERT INTO authorization_role_permissions(role_id,permission_id)
 SELECT r.id,p.id FROM authorization_roles r CROSS JOIN authorization_permissions p
 WHERE r.builtin AND r.slug='platform-operator' AND p.id LIKE 'invocation_security.%'
 ON CONFLICT DO NOTHING;
-- +goose Down
DELETE FROM authorization_role_permissions WHERE permission_id LIKE 'invocation_security.%';
DELETE FROM authorization_permissions WHERE id LIKE 'invocation_security.%';
