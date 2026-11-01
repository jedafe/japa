BEGIN;

CREATE OR REPLACE FUNCTION reconcile_credits_ledger() RETURNS bigint
    LANGUAGE plpgsql AS $fn$
DECLARE
    n bigint;
BEGIN
    WITH ins AS (
        INSERT INTO credits_ledger
            (id, seq, user_id, operation, amount, resulting_balance, source,
             actor_user_id, reference, idempotency_key, reason, created_at)
            OVERRIDING SYSTEM VALUE
            SELECT w.id, w.seq, w.user_id, w.operation, w.amount,
                   w.resulting_balance, w.source, w.actor_user_id,
                   w.reference, w.idempotency_key, w.reason, w.created_at
            FROM wallet_ledger w
            WHERE w.ledger_type = 'credits'
              AND NOT EXISTS (SELECT 1 FROM credits_ledger c WHERE c.id = w.id)
        RETURNING 1
    )
    SELECT count(*) INTO n FROM ins;

    PERFORM setval(pg_get_serial_sequence('credits_ledger', 'seq'),
                   (SELECT COALESCE(max(seq), 0) + 1 FROM credits_ledger), false);
    RETURN n;
END;
$fn$;

COMMIT;
