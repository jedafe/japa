-- Points balance floor — product rule: points never go negative.
-- Repair every user whose LATEST points balance is negative with a
-- compensating admin_adjust entry back to zero (keeps
-- SUM(amount) == resulting_balance chain integrity). The floor itself is
-- enforced in the service layer (floorPointsDebit) — a table CHECK is not
-- added because historical rows with negative resulting_balance must
-- remain as the immutable audit trail.

INSERT INTO wallet_ledger (user_id, ledger_type, operation, amount, resulting_balance, source, reference, idempotency_key, reason)
SELECT latest.user_id,
       'points',
       'admin_adjust',
       -latest.resulting_balance,
       0,
       'floor_correction',
       'floor_correction:' || latest.user_id::text,
       'floor_correction:' || latest.user_id::text,
       'Points balance floored at zero'
FROM (
    SELECT DISTINCT ON (user_id) user_id, resulting_balance
    FROM wallet_ledger
    WHERE ledger_type = 'points'
    ORDER BY user_id, seq DESC
) latest
WHERE latest.resulting_balance < 0;
