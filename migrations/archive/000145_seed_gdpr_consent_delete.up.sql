-- 000145_seed_gdpr_consent_delete.up.sql
-- Seeds the gdpr.consents.delete RBAC permission node and grants it to super_admin_role.
-- Backend for DELETE /admin/consents/{id} (clear/delete a GDPR consent record).

BEGIN;

INSERT INTO permissions (key, description) VALUES
    ('gdpr.consents.delete', 'Delete/clear a GDPR consent record (admin)')
ON CONFLICT (key) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'super_admin_role'
  AND p.key = 'gdpr.consents.delete'
ON CONFLICT DO NOTHING;

COMMIT;
