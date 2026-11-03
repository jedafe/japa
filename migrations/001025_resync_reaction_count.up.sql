-- Resync content_scores.reaction_count after migration 001023 deduped the
-- reactions table to one row per (entity_type, entity_id, user_id). The
-- denormalized counter still reflects the pre-dedupe row count; recompute it
-- from the live table so scoring matches what the live-counted card queries
-- already show.
UPDATE content_scores cs
SET reaction_count = rc.cnt
FROM (
    SELECT entity_id, COUNT(*)::int AS cnt
    FROM reactions
    WHERE entity_type = 'content'
    GROUP BY entity_id
) rc
WHERE cs.content_id = rc.entity_id
  AND cs.reaction_count IS DISTINCT FROM rc.cnt;

UPDATE content_scores cs
SET reaction_count = 0
WHERE cs.reaction_count <> 0
  AND NOT EXISTS (
      SELECT 1 FROM reactions r
      WHERE r.entity_type = 'content' AND r.entity_id = cs.content_id
  );
