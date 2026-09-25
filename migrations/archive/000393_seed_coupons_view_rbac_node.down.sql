-- 000393_seed_coupons_view_rbac_node.down.sql
-- Reverses 000393_seed_coupons_view_rbac_node.up.sql.

BEGIN;

DELETE FROM role_permissions
WHERE permission_id = (
    SELECT id FROM permissions WHERE key = 'coupons.view'
);

DELETE FROM permissions WHERE key = 'coupons.view';

COMMIT;
