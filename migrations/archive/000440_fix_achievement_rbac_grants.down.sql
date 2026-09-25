-- 000440_fix_achievement_rbac_grants.down.sql
BEGIN;
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE key IN
    ('achievement.create', 'achievement.edit_own', 'achievement.delete_own', 'achievement.moderate')
)
AND role_id IN (
    SELECT id FROM user_roles
    WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
      AND slug IN ('moderator', 'admin')
);
COMMIT;
