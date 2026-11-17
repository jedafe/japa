-- 001181_signer_renew_window.down.sql

BEGIN;

DELETE FROM system_settings WHERE key = 'stablecoin.signer_renew_window_days';

COMMIT;
