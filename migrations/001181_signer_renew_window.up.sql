-- 001181_signer_renew_window.up.sql
-- §128: rotation window for stablecoin:signer_scope_renew — the cron sweeps
-- only wallets whose nearest scope expires inside this many days (member-OTP
-- ceremony stays rare). Missed in the 001180 catalog seed.

BEGIN;

INSERT INTO system_settings (key, value) VALUES
    ('stablecoin.signer_renew_window_days', '30'::jsonb)
ON CONFLICT (key) DO NOTHING;

COMMIT;
