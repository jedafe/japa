-- 000253_gamification.down.sql
-- Reverses 000253_gamification.up.sql.

BEGIN;

-- Revoke gamification.* grants.
DELETE FROM role_permissions rp
USING user_roles ur, permissions p
WHERE rp.role_id = ur.id
  AND ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('super_admin', 'member')
  AND rp.permission_id = p.id
  AND p.key IN ('gamification.view', 'gamification.manage_badges', 'gamification.manage_challenges', 'gamification.award_badge');

-- Remove gamification.* permission nodes.
DELETE FROM permissions WHERE key IN ('gamification.view', 'gamification.manage_badges', 'gamification.manage_challenges', 'gamification.award_badge');

-- Remove gamification.* system_settings.
DELETE FROM system_settings
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND key IN ('gamification.badges_enabled', 'gamification.challenges_enabled', 'gamification.leaderboard_enabled');

-- Drop tables in reverse dependency order.
DROP TABLE IF EXISTS user_challenge_progress;
DROP TABLE IF EXISTS challenge_definitions;
DROP TABLE IF EXISTS user_badges;
DROP TABLE IF EXISTS badge_definitions;

COMMIT;
