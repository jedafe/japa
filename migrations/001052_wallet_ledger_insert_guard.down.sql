BEGIN;
DROP TRIGGER IF EXISTS wallet_ledger_balance_guard ON wallet_ledger;
DROP FUNCTION IF EXISTS wallet_ledger_balance_guard();
COMMIT;
