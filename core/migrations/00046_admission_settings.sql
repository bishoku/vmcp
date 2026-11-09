-- +goose Up
CREATE TABLE admission_settings (
 singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
 version bigint NOT NULL DEFAULT 1,
 settings jsonb NOT NULL
);
INSERT INTO admission_settings(settings) VALUES ('{"total":1024,"perServer":128,"perPrincipal":16}');
CREATE TRIGGER admission_settings_revision AFTER UPDATE ON admission_settings FOR EACH STATEMENT EXECUTE FUNCTION bump_config_revision();
INSERT INTO authorization_permissions(id,scopes,capture_restricted) VALUES
 ('admission.read',ARRAY['tenant'],false), ('admission.update',ARRAY['tenant'],false);
INSERT INTO authorization_role_permissions(role_id,permission_id)
 SELECT r.id,p.id FROM authorization_roles r CROSS JOIN authorization_permissions p
 WHERE r.builtin AND r.slug='platform-operator' AND p.id LIKE 'admission.%'
 ON CONFLICT DO NOTHING;
-- +goose Down
DELETE FROM authorization_role_permissions WHERE permission_id LIKE 'admission.%';
DELETE FROM authorization_permissions WHERE id LIKE 'admission.%';
DROP TABLE admission_settings;
