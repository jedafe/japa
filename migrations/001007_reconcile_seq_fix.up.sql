-- 001007 — reconcile_credits_ledger must not copy wallet_ledger.seq verbatim.
-- seq is an independent identity sequence per table; a verbatim copy can
-- collide with an existing credits_ledger seq (ambiguous seq DESC tail →
-- stale balance) or order a straggler behind newer rows. Let the identity
-- generate fresh seq so stragglers always sort as the newest rows.
BEGIN;

CREATE OR REPLACE FUNCTION reconcile_credits_ledger() RETURNS bigint
    LANGUAGE plpgsql AS $fn$
DECLARE
    n bigint;
BEGIN
    WITH ins AS (
        INSERT INTO credits_ledger
            (id, user_id, operation, amount, resulting_balance, source,
             actor_user_id, reference, idempotency_key, reason, created_at)
            SELECT w.id, w.user_id, w.operation, w.amount,
                   w.resulting_balance, w.source, w.actor_user_id,
                   w.reference, w.idempotency_key, w.reason, w.created_at
            FROM wallet_ledger w
            WHERE w.ledger_type = 'credits'
              AND NOT EXISTS (SELECT 1 FROM credits_ledger c WHERE c.id = w.id)
        RETURNING 1
    )
    SELECT count(*) INTO n FROM ins;
    RETURN n;
END;
$fn$;

COMMIT;
