-- 000998 — physical split of the credits stream out of wallet_ledger.
-- Ref: conventions/wallet-ledger-split-plan.md, §28, ADR-136.
--
-- wallet_ledger keeps the points stream (name retained — see the plan's
-- naming decision). credits_ledger takes the monetary stream with an
-- unconditional non-negative floor, refund/purchase linkage columns, and a
-- monotonic seq on both tables so the tail read can no longer tie on
-- transaction-stable created_at + random uuid ordering.
--
-- Historical credits rows are COPIED, not moved — the originals stay in
-- wallet_ledger as an immutable audit mirror until a later retirement
-- migration verifies parity and removes them.

BEGIN;

-- ── 1. Monotonic ordering column on wallet_ledger ──────────────────────
ALTER TABLE wallet_ledger ADD COLUMN IF NOT EXISTS seq bigint;

ALTER TABLE wallet_ledger DISABLE TRIGGER wallet_ledger_immutable;

WITH ranked AS (
    SELECT id, row_number() OVER (ORDER BY created_at, id) AS rn
    FROM wallet_ledger
)
UPDATE wallet_ledger w SET seq = r.rn FROM ranked r WHERE w.id = r.id;

ALTER TABLE wallet_ledger ALTER COLUMN seq SET NOT NULL;
ALTER TABLE wallet_ledger ENABLE TRIGGER wallet_ledger_immutable;
ALTER TABLE wallet_ledger ALTER COLUMN seq ADD GENERATED ALWAYS AS IDENTITY;
SELECT setval(pg_get_serial_sequence('wallet_ledger', 'seq'),
              (SELECT COALESCE(max(seq), 0) + 1 FROM wallet_ledger), false);

-- ── 2. credits_ledger ──────────────────────────────────────────────────
CREATE TABLE credits_ledger (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    seq               bigint GENERATED ALWAYS AS IDENTITY NOT NULL,
    user_id           uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    operation         text NOT NULL CHECK (operation = ANY (ARRAY[
                          'earn','spend','purchase','convert','refund',
                          'clawback','admin_adjust','withdraw'])),
    amount            bigint NOT NULL,
    resulting_balance bigint NOT NULL CHECK (resulting_balance >= 0),
    source            text,
    actor_user_id     uuid REFERENCES users(id) ON DELETE SET NULL,
    reference         text,
    idempotency_key   text UNIQUE,
    reason            text,
    payment_id        uuid REFERENCES payments(id),
    reverses_id       uuid REFERENCES credits_ledger(id),
    created_at        timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_credits_ledger_user_created
    ON credits_ledger (user_id, created_at DESC);
CREATE INDEX idx_credits_ledger_user_source
    ON credits_ledger (user_id, source);
CREATE INDEX idx_credits_ledger_idempotency
    ON credits_ledger (idempotency_key) WHERE idempotency_key IS NOT NULL;
CREATE INDEX idx_credits_ledger_payment
    ON credits_ledger (payment_id) WHERE payment_id IS NOT NULL;
CREATE INDEX idx_credits_ledger_reverses
    ON credits_ledger (reverses_id) WHERE reverses_id IS NOT NULL;

-- ── 3. Backfill the credits stream (ids + seq preserved) ───────────────
INSERT INTO credits_ledger
    (id, seq, user_id, operation, amount, resulting_balance, source,
     actor_user_id, reference, idempotency_key, reason, created_at)
    OVERRIDING SYSTEM VALUE
    SELECT id, seq, user_id, operation, amount, resulting_balance, source,
           actor_user_id, reference, idempotency_key, reason, created_at
    FROM wallet_ledger
    WHERE ledger_type = 'credits';

SELECT setval(pg_get_serial_sequence('credits_ledger', 'seq'),
              (SELECT COALESCE(max(seq), 0) + 1 FROM credits_ledger), false);

-- Recover purchase linkage from the credit_purchase:{payment_id}
-- idempotency-key convention (safe-cast via pattern match only).
UPDATE credits_ledger
SET payment_id = substring(idempotency_key from 'credit_purchase:(.+)')::uuid
WHERE idempotency_key ~ '^credit_purchase:[0-9a-fA-F-]{36}$';

-- ── 4. Append-only guard (installed after the backfill UPDATEs) ────────
CREATE TRIGGER credits_ledger_immutable
    BEFORE UPDATE OR DELETE ON credits_ledger
    FOR EACH ROW EXECUTE FUNCTION reject_mutation();

-- ── 5. Rolling-deploy catch-up ─────────────────────────────────────────
-- A stale binary can still write credits rows to wallet_ledger during a
-- rolling deploy. reconcile_credits_ledger() copies any stragglers across
-- (matched by row id) and bumps the identity sequence; run it after every
-- deploy until wallet_ledger stops receiving credits writes. Returns the
-- number of rows copied — a nonzero result is a signal, not an error.
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
