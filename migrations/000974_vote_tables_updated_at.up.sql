-- 000974_vote_tables_updated_at.up.sql
-- ADR-136: vote/reaction event references carry the transition timestamp so
-- every physical vote change produces a unique, replay-safe reference
-- without any best-effort seq counting. clock_timestamp() (not now()) so
-- serialized transactions on the same row can't share a value.

BEGIN;

ALTER TABLE comment_votes ADD COLUMN IF NOT EXISTS updated_at timestamptz;
ALTER TABLE forum_votes ADD COLUMN IF NOT EXISTS updated_at timestamptz;

-- Dependent-enqueue lookup (processed events enqueue events whose undoes
-- payload references them). Partial index — only pending events are
-- indexed, which is exactly the query filter.
CREATE INDEX IF NOT EXISTS idx_activity_events_undoes_pending
    ON activity_events ((payload->>'undoes_key'), (payload->>'undoes'))
    WHERE processed IS NOT TRUE;

COMMIT;
