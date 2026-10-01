-- 000761_onboarding_reminder_batch_limit
-- Seed the sweep's per-run candidate cap so it is visible/editable in
-- Control → Settings (same pattern as the other onboarding.reminder_*
-- keys from 000759). Code falls back to 2000 when absent/invalid.

BEGIN;

INSERT INTO system_settings (key, value, is_public)
SELECT 'onboarding.reminder_batch_limit', '2000'::jsonb, false
WHERE NOT EXISTS (
    SELECT 1 FROM system_settings WHERE key = 'onboarding.reminder_batch_limit'
);

COMMIT;
