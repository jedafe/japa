-- Revert 001000: restore the two-type CHECK, the conditional credits
-- floor, and copy any credits rows that only exist in credits_ledger back
-- into wallet_ledger (rows deleted by the up migration are restored with
-- their original ids/seq/created_at).
BEGIN;

ALTER TABLE wallet_ledger DROP CONSTRAINT wallet_ledger_ledger_type_check;
ALTER TABLE wallet_ledger
    ADD CONSTRAINT wallet_ledger_ledger_type_check
    CHECK (ledger_type = ANY (ARRAY['points'::text, 'credits'::text]));

ALTER TABLE wallet_ledger
    ADD CONSTRAINT credits_nonnegative
    CHECK (ledger_type <> 'credits' OR resulting_balance >= 0);

INSERT INTO wallet_ledger
    (id, seq, user_id, ledger_type, operation, amount, resulting_balance,
     source, actor_user_id, reference, idempotency_key, reason, created_at)
    OVERRIDING SYSTEM VALUE
    SELECT c.id, c.seq, c.user_id, 'credits', c.operation, c.amount,
           c.resulting_balance, c.source, c.actor_user_id, c.reference,
           c.idempotency_key, c.reason, c.created_at
    FROM credits_ledger c
    WHERE NOT EXISTS (SELECT 1 FROM wallet_ledger w WHERE w.id = c.id);

SELECT setval(pg_get_serial_sequence('wallet_ledger', 'seq'),
              (SELECT COALESCE(max(seq), 0) + 1 FROM wallet_ledger), false);

COMMIT;
