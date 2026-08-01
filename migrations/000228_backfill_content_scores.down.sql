-- 000228_backfill_content_scores.down.sql
-- The up migration only inserts zero-default rows that were missing. The
-- down migration removes only the zero-valued rows this migration created
-- (vote_score=0 AND comment_count=0 AND view_count=0 AND bookmark_count=0
-- AND velocity_score=0), preserving any rows that have since accumulated
-- real engagement data.

DELETE FROM content_scores
WHERE vote_score = 0
  AND comment_count = 0
  AND view_count = 0
  AND bookmark_count = 0
  AND velocity_score = 0;
