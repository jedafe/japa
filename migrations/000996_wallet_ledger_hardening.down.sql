-- Revert 000996 wallet ledger hardening.
DROP TRIGGER IF EXISTS wallet_ledger_immutable ON wallet_ledger;
DROP FUNCTION IF EXISTS reject_mutation();
ALTER TABLE wallet_ledger DROP CONSTRAINT IF EXISTS credits_nonnegative;
