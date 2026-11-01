-- Revert 000998. wallet_ledger still holds every historical credits row
-- (the split copied, never moved), so dropping credits_ledger loses only
-- rows written to it while the new code ran — those callers must be rolled
-- back first.
BEGIN;

DROP FUNCTION IF EXISTS reconcile_credits_ledger();
DROP TRIGGER IF EXISTS credits_ledger_immutable ON credits_ledger;
DROP TABLE IF EXISTS credits_ledger;

ALTER TABLE wallet_ledger ALTER COLUMN seq DROP IDENTITY IF EXISTS;
ALTER TABLE wallet_ledger DROP COLUMN IF EXISTS seq;

COMMIT;
