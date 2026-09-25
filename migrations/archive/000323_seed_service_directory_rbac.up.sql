-- 000323_seed_service_directory_rbac.up.sql
-- RBAC nodes for Service Directory (§98, ADR-106).
--   service.create    — agent, professional, org_member, admin
--   service.edit_own  — listing owner org
--   service.edit_any  — admin, super_admin
--   service.verify    — admin, super_admin
--   service.claim     — any registered org

BEGIN;

INSERT INTO permissions (key, description) VALUES
    ('service.create',   'Create a service listing (agent, professional, org_member, admin)'),
    ('service.edit_own', 'Edit own service listing (listing owner org)'),
    ('service.edit_any', 'Edit any service listing (admin+)'),
    ('service.verify',   'Verify service listings (admin+)'),
    ('service.claim',    'Claim a service listing (any registered org)')
ON CONFLICT (key) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('super_admin')
  AND p.key = 'service.create'
ON CONFLICT DO NOTHING;

-- org_member is a special role; grant create if it exists.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'member'
  AND p.key = 'service.create'
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('member', 'super_admin')
  AND p.key IN ('service.edit_own','service.claim')
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('super_admin')
  AND p.key IN ('service.edit_any','service.verify')
ON CONFLICT DO NOTHING;

COMMIT;
