-- 000627_seed_gamification_notification_types.up.sql
-- §64 Gamification (ADR-082) — seed the notification types dispatched by the
-- gamification service on badge awards and challenge completions so the
-- notification preference system can resolve them (Section 35).
--
-- Ref: japaconnect-engineering/sections/64-gamification.md
--      japaconnect-engineering/sections/35-notification-system.md

BEGIN;

INSERT INTO notification_types (key, label, description, default_in_app, default_email, default_push)
VALUES
    ('gamification.badge_awarded', 'Badge Awarded', 'You earned a community badge', true, false, false),
    ('gamification.challenge_completed', 'Challenge Completed', 'You completed a community challenge and earned the reward', true, false, false)
ON CONFLICT (key) DO NOTHING;

COMMIT;
