-- 000974_vote_tables_updated_at.up.sql
-- ADR-136: vote/reaction event references carry the transition timestamp so
-- every physical vote change produces a unique, replay-safe reference
-- without any best-effort seq counting. clock_timestamp() (not now()) so
-- serialized transactions on the same row can't share a value.

BEGIN;

ALTER TABLE comment_votes ADD COLUMN IF NOT EXISTS updated_at timestamptz;
ALTER TABLE forum_votes ADD COLUMN IF NOT EXISTS updated_at timestamptz;

COMMIT;
