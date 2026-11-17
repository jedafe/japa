-- 001184_stablecoin_minor_unit_settings.up.sql
-- §128: correct minor-unit settings that were seeded in whole-unit
-- magnitudes — the *_minor keys are 6-decimal USD-token minor, so the
-- 001180 seeds read as $0.005/day cap, $2.50 float cap, $0.0001 min
-- deposit. Correct values: $50/day per-member delegated cap (blast-radius
-- ceiling that still covers tips/purchases), $5,000 hot float (a few days
-- of launch-scale revenue; sweeps are a manual ceremony so a modest cap
-- just proposes sooner), $1.00 minimum deposit.
--
-- Ref: .jeos/sections/128-stablecoin-wallet-custody.md
-- Ref: .jeos/conventions/system-settings-catalog.md

BEGIN;

UPDATE system_settings SET value = '50000000'::jsonb
WHERE key = 'stablecoin.signer_daily_cap_usd_minor';

UPDATE system_settings SET value = '5000000000'::jsonb
WHERE key = 'stablecoin.hot_float_cap_minor';

UPDATE system_settings SET value = '1000000'::jsonb
WHERE key = 'stablecoin.min_deposit_minor';

COMMIT;
