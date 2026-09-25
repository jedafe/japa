-- 000418_seed_scam_alert_badge_notification.down.sql
-- Reverse: remove the Community Guardian badge and scam_alert.confirmed
-- notification type seeded by the up migration.

BEGIN;

-- Remove user_badges awarded for the community_guardian badge.
DELETE FROM user_badges
WHERE badge_id IN (
    SELECT id FROM badge_definitions WHERE slug = 'community_guardian' AND is_system = true
);

-- Remove the badge definitions.
DELETE FROM badge_definitions WHERE slug = 'community_guardian' AND is_system = true;

-- Remove the notification type.
DELETE FROM notification_types WHERE key = 'scam_alert.confirmed';

COMMIT;
