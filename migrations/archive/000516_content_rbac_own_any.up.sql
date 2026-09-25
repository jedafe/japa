-- 000516_content_rbac_own_any.up.sql
-- Fix content RBAC own/any split (BUG-001/BUG-002).
--
-- Section 14 and permission-key-naming.md require _own/_any scoping for
-- edit and delete. Migrations 000109/000176 removed the dot-scoped variants
-- and left only the flat content.edit / content.delete nodes, which were
-- never enforced with authorship checks. This migration adds the canonical
-- underscore-scoped nodes and grants them to the correct member+ roles.
--
-- Fixes:
--   - content.edit_own / content.edit_any
--   - content.delete_own / content.delete_any
--   - content.publish / content.schedule (also re-granted to member+ because
--     000175 used the wrong role slug 'member_role')

BEGIN;

INSERT INTO permissions (key, description) VALUES
    ('content.edit_own',  'Edit own content'),
    ('content.edit_any',  'Edit any content (moderator/admin)'),
    ('content.delete_own','Delete own content'),
    ('content.delete_any','Delete any content (moderator/admin)')
ON CONFLICT (key) DO NOTHING;

-- Self-service nodes for all member+ roles: edit/delete own, publish, schedule.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur
JOIN permissions p ON p.key IN (
    'content.edit_own',
    'content.delete_own',
    'content.publish',
    'content.schedule'
)
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('member', 'professional', 'agent', 'moderator', 'admin', 'super_admin')
ON CONFLICT DO NOTHING;

-- Moderator/admin/super-admin "any" nodes.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur
JOIN permissions p ON p.key IN (
    'content.edit_any',
    'content.delete_any'
)
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('moderator', 'admin', 'super_admin')
ON CONFLICT DO NOTHING;

COMMIT;
