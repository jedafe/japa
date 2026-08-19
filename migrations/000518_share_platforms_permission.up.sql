-- 000518_share_platforms_permission.up.sql
-- Adds a dedicated permission node for managing share platforms (BUG-009).
-- The admin share-platform endpoints previously reused
-- content.reaction_types.manage. Ref: Section 14, permission-key-naming.md.

BEGIN;

INSERT INTO permissions (key, description) VALUES
    ('content.share_platforms.manage', 'Manage share platforms (admin)')
ON CONFLICT (key) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur
JOIN permissions p ON p.key = 'content.share_platforms.manage'
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('admin', 'super_admin')
ON CONFLICT DO NOTHING;

COMMIT;
