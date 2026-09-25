-- 000500_studio_jobs_adr088_ai_columns.down.sql
BEGIN;

ALTER TABLE studio_jobs
    DROP COLUMN IF EXISTS hitl_reviewed_at,
    DROP COLUMN IF EXISTS hitl_reviewed_by,
    DROP COLUMN IF EXISTS ai_generated_at,
    DROP COLUMN IF EXISTS ai_model,
    DROP COLUMN IF EXISTS ai_generated;

COMMIT;
