-- 000655_scholarship_finder_public_settings.up.sql
-- Make scholarship_finder.* settings readable via GET /site/settings/public
-- so the Tools and Main scholarship surfaces can consume default_reminder_days
-- without an extra round-trip (Rule 3, Rule 16).
-- Ref: conventions/system-settings-catalog.md, sections/73-scholarship-finder.md

BEGIN;

UPDATE system_settings
SET is_public = true
WHERE key IN (
    'scholarship_finder.enabled',
    'scholarship_finder.default_reminder_days',
    'scholarship_finder.completion_points',
    'scholarship_finder.subdomain',
    'scholarship_finder.custom_domain'
);

COMMIT;
