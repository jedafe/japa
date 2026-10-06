-- 000798_roadmap_shadow_backfill.down.sql
-- Reverses the backfill: removes the shadow contents rows still linked from
-- roadmap items (the item's content_id is nulled by the ON DELETE SET NULL
-- FK). Comments on the shadow rows cascade via comments_content_id_fkey.

BEGIN;

DELETE FROM contents c
USING roadmap_items i
WHERE c.id = i.content_id
  AND c.content_type = 'roadmap'
  AND c.slug = 'roadmap-' || i.slug;

COMMIT;
