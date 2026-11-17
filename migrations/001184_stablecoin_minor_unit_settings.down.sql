-- 001184_stablecoin_minor_unit_settings.down.sql
-- Restore the (incorrect, whole-unit) seeds.

BEGIN;

UPDATE system_settings SET value = '5000'::jsonb
WHERE key = 'stablecoin.signer_daily_cap_usd_minor';

UPDATE system_settings SET value = '2500000'::jsonb
WHERE key = 'stablecoin.hot_float_cap_minor';

UPDATE system_settings SET value = '100'::jsonb
WHERE key = 'stablecoin.min_deposit_minor';

COMMIT;
