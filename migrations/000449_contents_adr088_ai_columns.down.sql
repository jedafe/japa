-- 000449_contents_adr088_ai_columns.down.sql
BEGIN;

ALTER TABLE contents
    DROP COLUMN IF EXISTS hitl_reviewed_at,
    DROP COLUMN IF EXISTS hitl_reviewed_by,
    DROP COLUMN IF EXISTS ai_generated_at,
    DROP COLUMN IF EXISTS ai_model,
    DROP COLUMN IF EXISTS ai_generated;

COMMIT;
