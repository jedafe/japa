-- 000759_onboarding_reminders (down)

BEGIN;

-- Remove dependent rows first: both notifications.notification_type and
-- notification_preferences.notification_type FK to notification_types(key).
DELETE FROM public.notification_preferences WHERE notification_type = 'onboarding_reminder';
DELETE FROM public.notifications WHERE notification_type = 'onboarding_reminder';
DELETE FROM public.notification_types WHERE key = 'onboarding_reminder';

DELETE FROM system_settings WHERE key IN (
    'onboarding.reminder_enabled',
    'onboarding.reminder_max_count',
    'onboarding.reminder_window_days',
    'onboarding.reminder_min_interval_hours'
);

ALTER TABLE onboarding_state
    DROP COLUMN IF EXISTS reminder_count,
    DROP COLUMN IF EXISTS last_reminder_at;

COMMIT;
