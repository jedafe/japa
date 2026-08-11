-- 000395_ai_hitl_reviews_source.down.sql
-- Reverses 000395: drops the source column + index from ai_hitl_reviews.

BEGIN;

DROP INDEX IF EXISTS idx_ai_hitl_reviews_source;
ALTER TABLE ai_hitl_reviews DROP COLUMN IF EXISTS source;

COMMIT;
