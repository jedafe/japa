BEGIN;

DELETE FROM system_settings WHERE key = 'wallet.max_daily_earn_credits';
DROP TRIGGER IF EXISTS credits_ledger_balance_guard ON credits_ledger;
DROP FUNCTION IF EXISTS credits_ledger_balance_guard();

COMMIT;
