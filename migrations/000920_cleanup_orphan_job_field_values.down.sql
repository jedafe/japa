-- Irreversible cleanup: the removed rows were duplicate writes from a bug,
-- not user data. Down is a no-op by design.
SELECT 1;
