-- 000319_seed_ama_rbac.up.sql
-- RBAC nodes for "Ask an Expat" Thread Format (§88, ADR-096).
--   forum.ama.create   — professional, agent, admin, super_admin
--   forum.ama.moderate — moderator, admin, super_admin

BEGIN;

INSERT INTO permissions (key, description) VALUES
    ('forum.ama.create',   'Create an AMA thread (professional, agent, admin+)'),
    ('forum.ama.moderate', 'Moderate AMA threads (moderator+)')
ON CONFLICT (key) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('super_admin')
  AND p.key = 'forum.ama.create'
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('super_admin')
  AND p.key = 'forum.ama.moderate'
ON CONFLICT DO NOTHING;

COMMIT;
