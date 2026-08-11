-- 000378_calendar_schema_fixes.down.sql
BEGIN;

-- Revert BUG-023: drop tenant-scoped index, recreate without tenant_id.
DROP INDEX IF EXISTS idx_calendar_subscriptions_unique;
CREATE UNIQUE INDEX IF NOT EXISTS idx_calendar_subscriptions_unique
    ON calendar_subscriptions(user_id, destination_space_id, COALESCE(category, '__all__'));

-- Revert BUG-003: make source_url nullable again.
ALTER TABLE calendar_entries ALTER COLUMN source_url DROP NOT NULL;

-- Revert BUG-002: migrate status values back.
UPDATE calendar_entries SET status = 'approved'  WHERE status = 'published';
UPDATE calendar_entries SET status = 'flagged'   WHERE status = 'unverified';

COMMIT;
