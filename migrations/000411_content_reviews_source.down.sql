-- 000411_content_reviews_source.down.sql
-- Reverses 000411_content_reviews_source.up.sql.

BEGIN;

DROP INDEX IF EXISTS idx_content_reviews_source;
ALTER TABLE content_reviews DROP COLUMN IF EXISTS source;

COMMIT;
