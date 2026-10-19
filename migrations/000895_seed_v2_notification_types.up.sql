-- 000895 — seed notification types used by v2 tasks that were never
-- registered. The notifications.notification_type FK rejects unseeded keys
-- (SQLSTATE 23503), so visa_timeline:prompt_update and
-- planning_group:checkin_reminder inserts were silently failing.
--
-- Ref: .jeos/sections/83-visa-timeline-tracker.md
--      .jeos/sections/82-japa-planning-groups.md

BEGIN;

INSERT INTO notification_types (key, label, description, default_in_app, default_email, default_push, category) VALUES
    ('visa_timeline.prompt_update', 'Visa timeline update reminder',
     'Weekly nudge to log the latest milestone on your in-progress visa timeline',
     true, false, false, 'system'),
    ('planning_group.checkin_reminder', 'Group check-in reminder',
     'Reminder to post a check-in for your japa planning group',
     true, false, false, 'social'),
    ('planning_group.mission_complete', 'Group milestone completed',
     'Your japa planning group completed a milestone',
     true, false, false, 'social')
ON CONFLICT (key) DO NOTHING;

COMMIT;
