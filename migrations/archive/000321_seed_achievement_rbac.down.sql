-- 000321_seed_achievement_rbac.down.sql
BEGIN;
DELETE FROM role_permissions WHERE permission_id IN (
    SELECT id FROM permissions WHERE key IN
    ('achievement.create','achievement.edit_own','achievement.delete_own','achievement.moderate'));
DELETE FROM permissions WHERE key IN
    ('achievement.create','achievement.edit_own','achievement.delete_own','achievement.moderate');
COMMIT;
