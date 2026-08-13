-- 000451_seed_affiliate_member_permissions.down.sql
-- Reverses 000451: removes affiliate.program.enroll and affiliate.links.manage
-- from member_role (the permission nodes themselves are retained).

BEGIN;

DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE key IN ('affiliate.program.enroll', 'affiliate.links.manage')
)
AND role_id IN (
    SELECT id FROM user_roles WHERE slug = 'member_role'
);

COMMIT;
