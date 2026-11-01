-- Wallet ledger hardening (ADR-136 follow-up).
-- Points and credits share wallet_ledger today; these constraints bring the
-- credits stream to bank-grade invariants without a table split:
--   1. credits rows can never have a negative resulting_balance
--      (points may — spam penalties legitimately drive points below zero)
--   2. every wallet_ledger row is immutable — corrections happen only via
--      compensating entries (admin_reversal / admin_adjust), never by
--      rewriting history.
-- Ref: conventions/wallet-ledger-split-plan.md (future credits_ledger split)

-- 1. Non-negative credits floor at the DB level.
ALTER TABLE wallet_ledger
    ADD CONSTRAINT credits_nonnegative
    CHECK (ledger_type <> 'credits' OR resulting_balance >= 0);

-- 2. Append-only ledger — shared reject function so other financial tables
--    (purchases, withdrawals) can adopt the same guard later.
CREATE OR REPLACE FUNCTION reject_mutation() RETURNS trigger
    LANGUAGE plpgsql AS $fn$
BEGIN
    RAISE EXCEPTION 'wallet_ledger rows are immutable: % not allowed', TG_OP;
END;
$fn$;

CREATE TRIGGER wallet_ledger_immutable
    BEFORE UPDATE OR DELETE ON wallet_ledger
    FOR EACH ROW EXECUTE FUNCTION reject_mutation();
