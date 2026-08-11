-- 000378_calendar_schema_fixes.up.sql
-- Fixes for Community Calendar (§86, ADR-094) audit findings:
--   BUG-002: status enum drift — approved→published, flagged→unverified
--   BUG-003: source_url must be NOT NULL (spec: "verifiable official source")
--   BUG-023: calendar_subscriptions unique index must include tenant_id
--
-- Ref: japaconnect-engineering/sections/86-community-calendar.md
--      japaconnect-engineering/adr/ADR-094-community-calendar.md

BEGIN;

-- BUG-002: Migrate status values to match §86 spec.
--   approved  → published
--   flagged   → unverified
--   (pending and archived are unchanged)
UPDATE calendar_entries SET status = 'published'  WHERE status = 'approved';
UPDATE calendar_entries SET status = 'unverified' WHERE status = 'flagged';

-- BUG-003: source_url is required per §86 ("Every entry must include a
-- verifiable official source link"). Set existing NULLs to a placeholder
-- before enforcing NOT NULL.
UPDATE calendar_entries SET source_url = '' WHERE source_url IS NULL;
ALTER TABLE calendar_entries ALTER COLUMN source_url SET NOT NULL;

-- BUG-023: calendar_subscriptions unique index must be tenant-scoped.
DROP INDEX IF EXISTS idx_calendar_subscriptions_unique;
CREATE UNIQUE INDEX IF NOT EXISTS idx_calendar_subscriptions_unique
    ON calendar_subscriptions(tenant_id, user_id, destination_space_id, COALESCE(category, '__all__'));

COMMIT;
