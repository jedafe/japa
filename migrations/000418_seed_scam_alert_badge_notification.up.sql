-- 000418_seed_scam_alert_badge_notification.up.sql
-- Seed the Community Guardian badge (§64) and the scam_alert.confirmed
-- notification type (§35) for the Scam Alert Board (§81, ADR-089).
--
-- The Community Guardian badge is awarded to members whose scam reports
-- are confirmed by moderators. It uses the activity_count trigger on
-- the `scam_alert.confirmed` activity key with a threshold of 1.
--
-- The scam_alert.confirmed notification type is dispatched to subscribers
-- of a destination when a report for that destination is confirmed.
--
-- Ref: japaconnect-engineering/sections/81-scam-alert-board.md
--      japaconnect-engineering/sections/64-gamification.md
--      japaconnect-engineering/sections/35-notification-system.md

BEGIN;

-- 1. Seed the scam_alert.confirmed notification type.
INSERT INTO notification_types (key, label, description, default_in_app, default_email, default_push)
VALUES ('scam_alert.confirmed', 'Scam Alert Confirmed', 'A scam alert for your subscribed destination was confirmed', true, false, false)
ON CONFLICT (key) DO NOTHING;

-- 2. Seed the Community Guardian badge for each existing tenant.
--    is_system = true so it cannot be deleted by admins.
--    trigger_type = 'activity_count', trigger_activity_key = 'scam_alert.confirmed',
--    trigger_value = 1 (awarded on first confirmed report).
INSERT INTO badge_definitions (tenant_id, slug, name, description, trigger_type, trigger_value, trigger_activity_key, is_active, is_system, display_order)
SELECT t.id, 'community_guardian', 'Community Guardian', 'Awarded when a scam report you submitted is confirmed by a moderator', 'activity_count', 1, 'scam_alert.confirmed', true, true, 60
FROM tenants t
WHERE NOT EXISTS (
    SELECT 1 FROM badge_definitions b
    WHERE b.tenant_id = t.id AND b.slug = 'community_guardian'
);

COMMIT;
