-- DB-layer enforcement of the points floor that a CHECK cannot express:
-- the constraint must apply to NEW rows while historical negative
-- resulting_balance rows remain as the immutable audit trail. A BEFORE
-- INSERT trigger recomputes amount/resulting_balance against the tail —
-- belt-and-suspenders behind the service-layer floorPointsDebit.
CREATE OR REPLACE FUNCTION wallet_ledger_points_floor() RETURNS trigger AS $$
DECLARE
    cur bigint;
BEGIN
    IF NEW.resulting_balance >= 0 THEN
        RETURN NEW;
    END IF;

    SELECT resulting_balance INTO cur
    FROM wallet_ledger
    WHERE user_id = NEW.user_id AND ledger_type = 'points'
    ORDER BY seq DESC LIMIT 1;

    cur := COALESCE(cur, 0);
    NEW.amount := -cur;
    NEW.resulting_balance := 0;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_wallet_ledger_points_floor
BEFORE INSERT ON wallet_ledger
FOR EACH ROW EXECUTE FUNCTION wallet_ledger_points_floor();
