-- 000423_calendar_subscriptions_unique_constraint.down.sql
-- Revert to the expression-based unique index from 000378.
BEGIN;

ALTER TABLE calendar_subscriptions
    DROP CONSTRAINT IF EXISTS calendar_subscriptions_tenant_user_dest_category_key;

CREATE UNIQUE INDEX IF NOT EXISTS idx_calendar_subscriptions_unique
    ON calendar_subscriptions(tenant_id, user_id, destination_space_id, COALESCE(category, '__all__'));

COMMIT;
