-- 000473_cv_format_sessions_adr088_columns.down.sql
BEGIN;

ALTER TABLE cv_format_sessions
    DROP COLUMN IF EXISTS hitl_review_notes,
    DROP COLUMN IF EXISTS hitl_reviewed_at,
    DROP COLUMN IF EXISTS hitl_reviewed_by,
    DROP COLUMN IF EXISTS ai_prompt_hash,
    DROP COLUMN IF EXISTS ai_generated;

COMMIT;
