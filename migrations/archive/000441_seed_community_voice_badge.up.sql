-- 000441_seed_community_voice_badge.up.sql
-- Seeds the "Community Voice" system badge (§64/§89) which is awarded when
-- a member shares 5 achievements in the Community Achievement Feed.
-- Also seeds the activity_point_rule for achievement.created so the wallet
-- can award points for each achievement post.
-- Ref: japaconnect-engineering/sections/64-gamification.md
--      japaconnect-engineering/sections/89-achievement-feed.md

BEGIN;

-- Community Voice badge: activity_count trigger on achievement.created, threshold 5.
INSERT INTO badge_definitions (tenant_id, slug, name, description, trigger_type, trigger_value, trigger_activity_key, is_active, is_system, display_order)
SELECT t.id, 'community_voice', 'Community Voice', 'Shared 5 achievements with the community', 'activity_count', 5, 'achievement.created', true, true, 70
FROM tenants t
WHERE NOT EXISTS (
    SELECT 1 FROM badge_definitions b
    WHERE b.tenant_id = t.id AND b.slug = 'community_voice'
);

-- Activity point rule for achievement.created (10 points per achievement post).
INSERT INTO activity_point_rules (tenant_id, activity_key, point_value, is_enabled, cap_per_day, cap_per_week, description)
SELECT t.id, 'achievement.created', 10, true, 3, 10, 'Points for sharing an achievement'
FROM tenants t
WHERE NOT EXISTS (
    SELECT 1 FROM activity_point_rules r
    WHERE r.tenant_id = t.id AND r.activity_key = 'achievement.created'
);

COMMIT;
