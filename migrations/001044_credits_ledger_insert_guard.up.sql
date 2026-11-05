-- 001044 — credits_ledger insert guard (SEC-W-010 hard boundary).
--
-- A BEFORE INSERT trigger that RECOMPUTES resulting_balance from the
-- locked tail row and enforces the non-negative floor. Callers can no
-- longer lie about the balance snapshot: whatever value is supplied is
-- discarded and derived inside the trigger. Combined with the immutable
-- UPDATE/DELETE trigger, forged history is now only possible via a row
-- whose AMOUNT is wrong — which the withdrawal-time provenance checks and
-- the hourly solvency sweep detect.
--
-- SELECT ... FOR UPDATE on the user's tail row serializes concurrent
-- inserts for that user at the DB level, so two racing inserts can't both
-- read the same previous balance.

BEGIN;

CREATE OR REPLACE FUNCTION credits_ledger_balance_guard() RETURNS trigger
    LANGUAGE plpgsql AS $fn$
DECLARE
    prev bigint;
BEGIN
    SELECT resulting_balance INTO prev
    FROM credits_ledger
    WHERE user_id = NEW.user_id
    ORDER BY seq DESC
    LIMIT 1
    FOR UPDATE;

    prev := COALESCE(prev, 0);

    IF prev + NEW.amount < 0 THEN
        RAISE EXCEPTION 'credits balance cannot go negative (user %: % + % < 0)',
            NEW.user_id, prev, NEW.amount;
    END IF;

    NEW.resulting_balance := prev + NEW.amount;
    RETURN NEW;
END;
$fn$;

CREATE TRIGGER credits_ledger_balance_guard
    BEFORE INSERT ON credits_ledger
    FOR EACH ROW EXECUTE FUNCTION credits_ledger_balance_guard();

-- Earn-side cap for credits (SEC-W-014): rolling-24h cap on positive
-- 'earn' credits a user may receive. 0 = disabled.
INSERT INTO system_settings (key, value)
VALUES ('wallet.max_daily_earn_credits', '0'::jsonb)
ON CONFLICT (key) DO NOTHING;

COMMIT;
