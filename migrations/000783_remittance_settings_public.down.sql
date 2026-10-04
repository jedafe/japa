-- 000783_remittance_settings_public.down.sql
-- Revert: hide the remittance settings from /site/settings/public again.

BEGIN;

UPDATE system_settings SET is_public = false
WHERE key IN (
    'remittance.default_send_currency',
    'remittance.manual_stale_hours',
    'remittance.subdomain',
    'remittance.custom_domain'
);

COMMIT;
