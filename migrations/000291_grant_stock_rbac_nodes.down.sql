-- 000291_grant_stock_rbac_nodes.down.sql
-- Reverses 000291: removes the role_permissions grants and the media.stock.admin
-- permission node. The original three nodes (media.stock.search/import/providers)
-- remain — they are owned by migration 000187 and removed by its down migration.

BEGIN;

-- Remove the grants added by 000291.
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions
    WHERE key IN (
        'media.stock.search',
        'media.stock.import',
        'media.stock.providers',
        'media.stock.admin'
    )
)
AND role_id IN (
    SELECT id FROM user_roles
    WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
      AND slug IN ('member', 'super_admin')
);

-- Remove the admin node (owned by this migration).
DELETE FROM permissions WHERE key = 'media.stock.admin';

COMMIT;
