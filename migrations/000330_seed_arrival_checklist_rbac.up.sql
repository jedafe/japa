-- 000330_seed_arrival_checklist_rbac.up.sql
-- RBAC nodes for Arrival Checklist (§92, ADR-100).
--   arrival_checklist.manage — admin, super_admin

BEGIN;

INSERT INTO permissions (key, description) VALUES
    ('arrival_checklist.manage', 'Manage arrival checklist guides (admin+)')
ON CONFLICT (key) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('admin', 'super_admin')
  AND p.key = 'arrival_checklist.manage'
ON CONFLICT DO NOTHING;

COMMIT;
