-- 001066_scholarship_cfv_content_bucket.up.sql
-- Unify the scholarship cfv bucket: scholarship entities are guide content
-- rows, so customfields.ValueEntityType now normalizes 'scholarship' →
-- 'content'. Move any pre-existing entity_type='scholarship' rows so they
-- remain readable by the scholarships service (which reads 'content' +
-- contents.field_values) and by wiki display.
--
-- Collision rule: a 'content' row for the same (field_id, entity_id)
-- already canonical wins — the leftover 'scholarship' duplicate is deleted.

BEGIN;

UPDATE custom_field_values v
SET entity_type = 'content', updated_at = now()
WHERE v.entity_type = 'scholarship'
  AND NOT EXISTS (
      SELECT 1 FROM custom_field_values c
      WHERE c.field_id = v.field_id
        AND c.entity_type = 'content'
        AND c.entity_id = v.entity_id
  );

DELETE FROM custom_field_values
WHERE entity_type = 'scholarship';

COMMIT;
