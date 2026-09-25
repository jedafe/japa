-- 000239_add_ad_network_manage_permission.down.sql
-- Remove the ad.network.manage permission node and its super_admin grant.
BEGIN;
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE key = 'ad.network.manage'
);

DELETE FROM permissions
WHERE key = 'ad.network.manage';
COMMIT;
