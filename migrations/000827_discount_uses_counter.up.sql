-- 000827_discount_uses_counter.up.sql — atomic discount max_uses enforcement.
-- Adds a uses counter to discount_codes so checkouts consume a use with a
-- single guarded UPDATE ... WHERE uses < max_uses RETURNING instead of a
-- racy read-check (COUNT) followed by an unguarded insert. Concurrent
-- checkouts serialize on the row lock, so max_uses can never be overshot.
-- Backfills the counter from existing discount_code_uses rows.

ALTER TABLE discount_codes ADD COLUMN IF NOT EXISTS uses integer NOT NULL DEFAULT 0;

UPDATE discount_codes dc
SET uses = COALESCE(u.cnt, 0)
FROM (
    SELECT code_id, COUNT(*)::int AS cnt
    FROM discount_code_uses
    GROUP BY code_id
) u
WHERE u.code_id = dc.id;
