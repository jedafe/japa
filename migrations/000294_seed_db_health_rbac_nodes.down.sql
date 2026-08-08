-- 000294_seed_db_health_rbac_nodes.down.sql
-- Reverses 000294: removes the role_permissions grants and the two
-- platform.db.* permission nodes.

BEGIN;

-- Remove the grants added by 000294.
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions
    WHERE key IN ('platform.db.health_view', 'platform.db.vacuum_trigger')
)
AND role_id IN (
    SELECT id FROM user_roles
    WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
      AND slug = 'super_admin'
);

-- Remove the permission nodes (owned by this migration).
DELETE FROM permissions WHERE key IN ('platform.db.health_view', 'platform.db.vacuum_trigger');

COMMIT;
