-- 000627_seed_gamification_notification_types.down.sql
-- Reverse: remove the gamification notification types seeded by the up
-- migration (Section 64).

BEGIN;

DELETE FROM notification_types
WHERE key IN ('gamification.badge_awarded', 'gamification.challenge_completed');

COMMIT;
