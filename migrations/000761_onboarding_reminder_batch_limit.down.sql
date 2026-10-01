-- 000761_onboarding_reminder_batch_limit (down)

BEGIN;

DELETE FROM system_settings WHERE key = 'onboarding.reminder_batch_limit';

COMMIT;
