-- 000424_drop_orphaned_tag_rbac_nodes.down.sql
-- Revert BUG-004: re-seed the three orphaned tag permission nodes and restore
-- their prior grants to the member role (000099/000109) and super_admin_role
-- (000125). These nodes are not used by the server (all admin tag operations
-- are gated on `tags.manage`), but re-seeding them restores the pre-migration
-- RBAC row count for environments that rolled back.

BEGIN;

-- 1. Re-seed the three permission nodes.
INSERT INTO permissions (key, description) VALUES
    ('tags.create', 'Create general tags'),
    ('tags.edit', 'Edit general tags'),
    ('tags.delete', 'Delete general tags')
ON CONFLICT (key) DO NOTHING;

-- 2. Restore the member-role grant for tags.create (000099/000109).
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'member'
  AND p.key = 'tags.create'
ON CONFLICT DO NOTHING;

-- 3. Restore the super_admin_role grants for all three nodes (000125).
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'super_admin_role'
  AND p.key IN ('tags.create', 'tags.edit', 'tags.delete')
ON CONFLICT DO NOTHING;

COMMIT;
