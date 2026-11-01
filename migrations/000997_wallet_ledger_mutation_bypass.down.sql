-- Revert 000997: restore the unconditional immutability guard.
CREATE OR REPLACE FUNCTION reject_mutation() RETURNS trigger
    LANGUAGE plpgsql AS $fn$
BEGIN
    RAISE EXCEPTION 'wallet_ledger rows are immutable: % not allowed', TG_OP;
END;
$fn$;
