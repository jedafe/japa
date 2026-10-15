-- 000868_col_calculator_settings_public.up.sql
-- Expose the col_calculator settings consumed by public surfaces:
--   col_calculator.max_cities      — destination-city cap shown in the
--     calculator UI (main + tools frontends, §72).
--   col_calculator.stale_days      — drives the "data may be outdated"
--     notice (§72).
--   col_calculator.subdomain / col_calculator.custom_domain — tool host
--     detection in the tools app hooks (same is_public treatment as
--     fx/remittance, migration 000783).
--
-- Ref: .jeos/conventions/system-settings-catalog.md
--      .jeos/sections/72-cost-of-living.md

BEGIN;

UPDATE system_settings SET is_public = true
WHERE key IN (
    'col_calculator.max_cities',
    'col_calculator.stale_days',
    'col_calculator.subdomain',
    'col_calculator.custom_domain'
);

COMMIT;
