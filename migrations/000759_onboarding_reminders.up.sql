-- 000759_onboarding_reminders
-- Reminder bookkeeping for incomplete onboarding (Section 09): the daily
-- onboarding:reminder_sweep task (§11) nudges newly registered users who
-- haven't finished the wizard. reminder_count bounds sends; the
-- onboarding_reminder notification type gates user preference overrides.

BEGIN;

ALTER TABLE onboarding_state
    ADD COLUMN reminder_count int NOT NULL DEFAULT 0,
    ADD COLUMN last_reminder_at timestamptz;

INSERT INTO public.notification_types
    (key, label, description, enabled, default_in_app, default_email, default_push, category)
SELECT 'onboarding_reminder', 'Onboarding Reminder',
       'Reminder to finish setting up your account',
       true, true, false, false, 'system'
WHERE NOT EXISTS (
    SELECT 1 FROM public.notification_types WHERE key = 'onboarding_reminder'
);

-- Seed the sweep's tuning knobs so they are visible/editable in
-- Control → Settings (same pattern as onboarding.tour_steps, 000757).
INSERT INTO system_settings (key, value, is_public)
SELECT k, v::jsonb, false
FROM (VALUES
    ('onboarding.reminder_enabled',            'true'),
    ('onboarding.reminder_max_count',          '5'),
    ('onboarding.reminder_window_days',        '14'),
    ('onboarding.reminder_min_interval_hours', '20')
) AS s(k, v)
WHERE NOT EXISTS (
    SELECT 1 FROM system_settings WHERE system_settings.key = s.k
);

COMMIT;
