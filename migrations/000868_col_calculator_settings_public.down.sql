-- 000868_col_calculator_settings_public.down.sql
-- Revert: hide the col_calculator settings from /site/settings/public again.

BEGIN;

UPDATE system_settings SET is_public = false
WHERE key IN (
    'col_calculator.max_cities',
    'col_calculator.stale_days',
    'col_calculator.subdomain',
    'col_calculator.custom_domain'
);

COMMIT;
