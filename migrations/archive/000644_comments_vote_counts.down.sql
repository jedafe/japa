-- 000644_comments_vote_counts.down.sql
-- Remove denormalized upvote/downvote counters from comments.

BEGIN;

ALTER TABLE comments
    DROP COLUMN IF EXISTS upvote_count,
    DROP COLUMN IF EXISTS downvote_count;

COMMIT;
