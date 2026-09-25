-- 000573_template_literals_permission.up.sql
-- Adds RBAC permission for managing template literals.

BEGIN;

INSERT INTO permissions (key, description) VALUES
    ('content.literals.manage', 'Manage global template literals (admin)')
ON CONFLICT (key) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur
JOIN permissions p ON p.key = 'content.literals.manage'
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('admin', 'super_admin')
ON CONFLICT DO NOTHING;

COMMIT;
