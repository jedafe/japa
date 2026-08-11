-- Section 86: Community Calendar reminder notification type
-- The calendar service dispatches this notification type when the
-- calendar:send_reminders Asynq task fires at 90/30/7-day marks before
-- each published entry. The notifications table has a FK to
-- notification_types(key), so this must be seeded before the dispatch
-- code can insert rows.
-- Ref: japaconnect-engineering/sections/86-community-calendar.md
-- Ref: japaconnect-engineering/sections/35-notification-system.md

INSERT INTO notification_types (key, label, description, default_in_app, default_email, default_push) VALUES
    ('calendar_reminder', 'Calendar Reminder',
     'Reminder for an upcoming calendar event you subscribed to',
     true, true, false)
ON CONFLICT (key) DO NOTHING;
