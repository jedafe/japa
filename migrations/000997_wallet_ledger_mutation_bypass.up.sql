-- Allow a scoped bypass of the ledger-immutability trigger.
--
-- The 000996 wallet_ledger_immutable trigger fires on UPDATE and DELETE —
-- including ON DELETE CASCADE / ON DELETE SET NULL actions from users.
-- That made GDPR hard erasure impossible for any user with ledger rows
-- (every active user earns points): DELETE FROM users cascaded into
-- wallet_ledger deletes, the trigger raised, and the erasure tx rolled
-- back. Ledger rows must still be immutable to application code, so the
-- bypass is a deliberately awkward opt-in GUC that only the erasure path
-- (and migrations, via SET inside the migration session) ever sets:
--   SET LOCAL wallet.allow_ledger_mutation = 'on'
-- inside the transaction performing the delete. Missing/empty/off all
-- keep the guard armed.
CREATE OR REPLACE FUNCTION reject_mutation() RETURNS trigger
    LANGUAGE plpgsql AS $fn$
BEGIN
    IF current_setting('wallet.allow_ledger_mutation', true) = 'on' THEN
        IF TG_OP = 'DELETE' THEN
            RETURN OLD;
        END IF;
        RETURN NEW;
    END IF;
    RAISE EXCEPTION '% rows are immutable: % not allowed', TG_TABLE_NAME, TG_OP;
END;
$fn$;
