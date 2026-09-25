-- 000194_seed_affiliate_member_rbac_nodes.down.sql
-- Reverses 000194: removes the member-scope affiliate.* node grants and nodes.

BEGIN;

DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions
    WHERE key IN ('affiliate.program.enroll', 'affiliate.links.manage')
);

DELETE FROM permissions
WHERE key IN ('affiliate.program.enroll', 'affiliate.links.manage');

COMMIT;
