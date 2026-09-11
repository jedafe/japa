BEGIN;

UPDATE system_settings
SET is_public = false
WHERE key IN (
    'scholarship_finder.enabled',
    'scholarship_finder.default_reminder_days',
    'scholarship_finder.completion_points',
    'scholarship_finder.subdomain',
    'scholarship_finder.custom_domain'
);

COMMIT;
