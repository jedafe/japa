-- 000609_seed_tier_roles.down.sql
-- Reverses the pro_member / business_member role seed.

BEGIN;

DELETE FROM group_roles
WHERE group_id IN (
    SELECT id FROM user_groups
    WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug IN ('pro', 'business')
)
AND role_id IN (
    SELECT id FROM user_roles
    WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug IN ('pro_member', 'business_member')
);

DELETE FROM role_permissions
WHERE role_id IN (
    SELECT id FROM user_roles
    WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug IN ('pro_member', 'business_member')
);

DELETE FROM user_roles
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND slug IN ('pro_member', 'business_member');

COMMIT;
