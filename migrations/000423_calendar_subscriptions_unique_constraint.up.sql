-- 000423_calendar_subscriptions_unique_constraint.up.sql
-- Replace the expression-based unique index on calendar_subscriptions with a
-- plain unique index using NULLS NOT DISTINCT, so that
-- ON CONFLICT (tenant_id, user_id, destination_space_id, category) works in
-- the store's Subscribe upsert. The old index (000314/000378) used
-- COALESCE(category,'__all__'), which ON CONFLICT cannot target by naming the
-- raw `category` column. NULLS NOT DISTINCT (PG15+) ensures two rows with NULL
-- category on the same (tenant, user, destination_space) conflict, matching
-- the "nullable = all categories" semantics from §86.
--
-- Note: NULLS NOT DISTINCT is supported in CREATE [UNIQUE] INDEX on PG15+, but
-- NOT in ALTER TABLE ... ADD CONSTRAINT UNIQUE (...) (syntax error), so we use
-- a unique index here. ON CONFLICT (col, ...) matches a unique index on exactly
-- those columns.
--
-- Ref: japaconnect-engineering/sections/86-community-calendar.md
--      internal/calendar/store.go Subscribe

BEGIN;

-- Drop the old expression-based unique index.
DROP INDEX IF EXISTS idx_calendar_subscriptions_unique;

-- Plain unique index with NULLS NOT DISTINCT so NULL categories conflict
-- with each other (matching the old COALESCE('__all__') intent).
CREATE UNIQUE INDEX IF NOT EXISTS idx_calendar_subscriptions_unique
    ON calendar_subscriptions (tenant_id, user_id, destination_space_id, category)
    NULLS NOT DISTINCT;

COMMIT;
