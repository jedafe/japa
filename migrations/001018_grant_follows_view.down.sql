-- 001018_grant_follows_view.down.sql
-- Removes the follows.view grants added by the up migration. The
-- permission node itself predates this migration (seeded by 000109) and is
-- left in place.

BEGIN;

DELETE FROM role_permissions
WHERE role_id IN (
    SELECT id FROM user_roles
    WHERE slug IN ('member', 'pro_member', 'business_member')
)
AND permission_id IN (
    SELECT id FROM permissions
    WHERE key = 'follows.view'
);

COMMIT;
