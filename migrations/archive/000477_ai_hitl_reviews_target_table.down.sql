-- 000477_ai_hitl_reviews_target_table.down.sql
BEGIN;

DROP INDEX IF EXISTS idx_ai_hitl_reviews_target_table;
ALTER TABLE ai_hitl_reviews DROP COLUMN IF EXISTS target_table;

COMMIT;
