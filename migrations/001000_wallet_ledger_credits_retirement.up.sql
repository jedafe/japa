-- 001000 — retire the mirrored credits rows from wallet_ledger.
-- Ref: conventions/wallet-ledger-split-plan.md (final phase).
--
-- Post-000998 every credits write goes to credits_ledger; the historical
-- copies in wallet_ledger were kept as an audit mirror while the cutover
-- bedded in. This migration reconciles any stragglers, verifies parity,
-- deletes the mirror under the immutability bypass, then pins
-- wallet_ledger to 'points' so a wrong-table write fails loudly instead
-- of silently split-braining.

BEGIN;

-- Copy across anything a stale binary wrote during the deploy window
-- (no-op when the dispatcher is already the only writer).
SELECT reconcile_credits_ledger();

-- Parity gate: every wallet_ledger credits row must have a credits_ledger
-- twin by id, or the mirror is incomplete and we stop rather than lose rows.
DO $$
DECLARE
    missing bigint;
BEGIN
    SELECT count(*) INTO missing
    FROM wallet_ledger w
    WHERE w.ledger_type = 'credits'
      AND NOT EXISTS (SELECT 1 FROM credits_ledger c WHERE c.id = w.id);
    IF missing > 0 THEN
        RAISE EXCEPTION 'credits retirement aborted: % wallet_ledger credits rows lack a credits_ledger twin', missing;
    END IF;
END
$$;

-- The immutable trigger guards application code, not erasure/migrations —
-- same bypass the GDPR path uses (000997).
SET LOCAL wallet.allow_ledger_mutation = 'on';

DELETE FROM wallet_ledger WHERE ledger_type = 'credits';

-- Pin the table to the points stream. From here, an INSERT with
-- ledger_type='credits' violates the CHECK — loud, not silent.
ALTER TABLE wallet_ledger DROP CONSTRAINT wallet_ledger_ledger_type_check;
ALTER TABLE wallet_ledger
    ADD CONSTRAINT wallet_ledger_ledger_type_check CHECK (ledger_type = 'points');

-- The per-type floor is vacuous on a points-only table (penalties may
-- legitimately take points negative); the unconditional floor lives on
-- credits_ledger now.
ALTER TABLE wallet_ledger DROP CONSTRAINT credits_nonnegative;

COMMIT;
