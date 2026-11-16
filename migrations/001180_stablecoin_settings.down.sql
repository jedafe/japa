-- 001180_stablecoin_settings.down.sql

BEGIN;

DELETE FROM system_settings WHERE key IN (
    'stablecoin.network',
    'stablecoin.assets',
    'stablecoin.signer_daily_cap_usd_minor',
    'stablecoin.signer_scope_expiry_days',
    'stablecoin.delegated_signer_ref',
    'stablecoin.escrow_arbiter_ref',
    'stablecoin.tip_fee_bps',
    'stablecoin.platform_share_bps',
    'stablecoin.withdrawal_fee_bps',
    'stablecoin.hot_float_cap_minor',
    'stablecoin.escrow_confirm_hours',
    'stablecoin.signup_bonus_credits',
    'stablecoin.min_deposit_minor'
);

COMMIT;
