-- +goose Up
-- Keep immutable role IDs and all assignments. A permission bundle is not an
-- administrative identity; only server-owned admin identities manage access.
DELETE FROM authorization_role_permissions p USING authorization_roles r
WHERE p.role_id=r.id AND r.builtin AND r.slug='platform-admin' AND p.permission_id LIKE 'access.%';
UPDATE authorization_roles SET slug='platform-operator',name='Platform Operator',
 description='Broad operational permissions, including audit payloads within assigned capture scopes. Does not grant platform administrator identity or access management.',version=version+1
WHERE builtin AND slug='platform-admin';

-- +goose Down
UPDATE authorization_roles SET slug='platform-admin',name='Platform Admin',description='',version=greatest(1,version-1)
WHERE builtin AND slug='platform-operator';
INSERT INTO authorization_role_permissions(role_id,permission_id)
SELECT r.id,p.id FROM authorization_roles r CROSS JOIN authorization_permissions p
WHERE r.builtin AND r.slug='platform-admin' AND p.id LIKE 'access.%'
ON CONFLICT DO NOTHING;
