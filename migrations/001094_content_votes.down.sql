-- 001094_content_votes.down.sql
-- Drop the content vote surface and the contents counter columns.

BEGIN;

DROP TABLE IF EXISTS content_votes;

ALTER TABLE contents
    DROP COLUMN IF EXISTS vote_score,
    DROP COLUMN IF EXISTS upvote_count,
    DROP COLUMN IF EXISTS downvote_count;

COMMIT;
