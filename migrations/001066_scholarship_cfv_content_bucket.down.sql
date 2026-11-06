-- 001066_scholarship_cfv_content_bucket.down.sql
-- Reverse: move 'content' cfv rows written against wiki_scholarship_* field
-- defs back to entity_type='scholarship'. Rows written by guide fields were
-- always 'content' and stay put — this restores the pre-normalization
-- buckets for the wiki-bound scholarship fields only.

BEGIN;

UPDATE custom_field_values v
SET entity_type = 'scholarship', updated_at = now()
FROM custom_fields f
JOIN custom_field_groups g ON g.id = f.group_id
WHERE v.field_id = f.id
  AND g.slug LIKE 'wiki_scholarship_%'
  AND v.entity_type = 'content'
  AND NOT EXISTS (
      SELECT 1 FROM custom_field_values c
      WHERE c.field_id = v.field_id
        AND c.entity_type = 'scholarship'
        AND c.entity_id = v.entity_id
  );

COMMIT;
