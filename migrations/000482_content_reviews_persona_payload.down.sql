-- 000482_content_reviews_persona_payload.down.sql
-- Reverse of 000482_content_reviews_persona_payload.up.sql.
-- Drops the persona_payload jsonb column. Existing persona_action rows retain
-- their feedback text column (untouched by the up migration).

BEGIN;

DROP INDEX IF EXISTS idx_content_reviews_persona_payload;
ALTER TABLE content_reviews DROP COLUMN IF EXISTS persona_payload;

COMMIT;
