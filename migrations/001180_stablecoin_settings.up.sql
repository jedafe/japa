-- 001180_stablecoin_settings.up.sql
-- §128: seed the stablecoin.* system_settings keys so they appear in
-- Control → System Settings. Values match the catalog defaults; the
-- service falls back to these values when a row is absent, so seeding
-- is for admin visibility, not correctness.
--
-- Ref: .jeos/sections/128-stablecoin-wallet-custody.md
-- Ref: .jeos/conventions/system-settings-catalog.md

BEGIN;

INSERT INTO system_settings (key, value) VALUES
    ('stablecoin.network',                    '"solana"'::jsonb),
    ('stablecoin.assets',                     '["usdc","usdt"]'::jsonb),
    ('stablecoin.signer_daily_cap_usd_minor', '50000000'::jsonb),
    ('stablecoin.signer_scope_expiry_days',   '365'::jsonb),
    ('stablecoin.delegated_signer_ref',       '""'::jsonb),
    ('stablecoin.escrow_arbiter_ref',         '""'::jsonb),
    ('stablecoin.tip_fee_bps',                '500'::jsonb),
    ('stablecoin.platform_share_bps',         '1500'::jsonb),
    ('stablecoin.withdrawal_fee_bps',         '100'::jsonb),
    ('stablecoin.hot_float_cap_minor',        '5000000000'::jsonb),
    ('stablecoin.escrow_confirm_hours',       '72'::jsonb),
    ('stablecoin.signup_bonus_credits',       '500'::jsonb),
    ('stablecoin.min_deposit_minor',          '1000000'::jsonb)
ON CONFLICT (key) DO NOTHING;

COMMIT;
