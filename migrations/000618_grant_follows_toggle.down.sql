-- 000618_grant_follows_toggle.down.sql
-- Removes the follows.toggle grants added by the up migration. The
-- permission node itself predates this migration (seeded by 000109) and is
-- left in place.

BEGIN;

DELETE FROM role_permissions
WHERE role_id IN (
    SELECT id FROM user_roles
    WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
      AND slug IN ('member', 'pro_member', 'business_member')
)
AND permission_id IN (
    SELECT id FROM permissions
    WHERE key = 'follows.toggle'
);

COMMIT;
