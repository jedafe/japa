-- 000441_seed_community_voice_badge.down.sql
BEGIN;
DELETE FROM badge_definitions WHERE slug = 'community_voice' AND is_system = true;
DELETE FROM activity_point_rules WHERE activity_key = 'achievement.created';
COMMIT;
