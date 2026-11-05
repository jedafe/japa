-- 001052 — points ledger insert guard (SEC-W-017).
--
-- Sibling of 001044's credits_ledger guard: BEFORE INSERT recompute of
-- resulting_balance from the locked tail row + hard floor at 0.
-- Balances never go negative — clawback is clamped Go-side (tasks.go),
-- this trigger is the DB-level guarantee for every write path.
--
-- wallet_ledger new writes are points-only (credits route to
-- credits_ledger via the ledger dispatcher); the trigger also keys the
-- tail lookup on ledger_type for correctness on mixed rows.

BEGIN;

CREATE OR REPLACE FUNCTION wallet_ledger_balance_guard() RETURNS trigger
    LANGUAGE plpgsql AS $fn$
DECLARE
    prev bigint;
BEGIN
    SELECT resulting_balance INTO prev
    FROM wallet_ledger
    WHERE user_id = NEW.user_id
      AND ledger_type = NEW.ledger_type
    ORDER BY seq DESC
    LIMIT 1
    FOR UPDATE;

    prev := COALESCE(prev, 0);

    IF prev + NEW.amount < 0 THEN
        RAISE EXCEPTION 'points balance cannot go negative (user %: % + % < 0)',
            NEW.user_id, prev, NEW.amount;
    END IF;

    NEW.resulting_balance := prev + NEW.amount;
    RETURN NEW;
END;
$fn$;

CREATE TRIGGER wallet_ledger_balance_guard
    BEFORE INSERT ON wallet_ledger
    FOR EACH ROW EXECUTE FUNCTION wallet_ledger_balance_guard();

COMMIT;
