-- 000644_comments_vote_counts.up.sql
-- Add denormalized upvote/downvote counters to comments and backfill them from
-- comment_votes. These counters power the daily.dev-style "N upvotes" label in
-- the unified comment engine (Section 49 amendment).

BEGIN;

ALTER TABLE comments
    ADD COLUMN upvote_count   int NOT NULL DEFAULT 0,
    ADD COLUMN downvote_count int NOT NULL DEFAULT 0;

UPDATE comments c
SET
    upvote_count = COALESCE((
        SELECT COUNT(*)::int
        FROM comment_votes cv
        WHERE cv.comment_id = c.id
          AND cv.tenant_id = c.tenant_id
          AND cv.value = 1
    ), 0),
    downvote_count = COALESCE((
        SELECT COUNT(*)::int
        FROM comment_votes cv
        WHERE cv.comment_id = c.id
          AND cv.tenant_id = c.tenant_id
          AND cv.value = -1
    ), 0),
    vote_score = COALESCE((
        SELECT SUM(cv.value)::int
        FROM comment_votes cv
        WHERE cv.comment_id = c.id
          AND cv.tenant_id = c.tenant_id
    ), 0)
WHERE c.deleted_at IS NULL;

COMMIT;
