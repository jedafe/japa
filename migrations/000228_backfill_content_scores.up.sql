-- 000228_backfill_content_scores.up.sql
-- Backfill content_scores rows for published content that was created before
-- the score-row-on-publish fix. Without a content_scores row, INNER JOIN feed
-- queries (explore/top/rising/space/profile) exclude the content entirely,
-- and the engagement worker cannot increment vote_score/bookmark_count.
--
-- The row is created with zero defaults; the engagement worker and the
-- velocity-score cron populate the columns as events arrive. Existing rows
-- are preserved (ON CONFLICT DO NOTHING).
--
-- Ref: Section 32 (Activity Feed / Timeline), Guide §21 (stub/missing data).

INSERT INTO content_scores (content_id)
SELECT c.id
FROM contents c
WHERE c.status = 'published'
  AND c.deleted_at IS NULL
  AND NOT EXISTS (
      SELECT 1 FROM content_scores cs WHERE cs.content_id = c.id
  )
ON CONFLICT (content_id) DO NOTHING;
