-- 000457_calendar_adr088_ai_columns.down.sql
-- Reverses 000457_calendar_adr088_ai_columns.up.sql.
-- Drops the ADR-088 AI generation columns from calendar_entries.

BEGIN;

ALTER TABLE calendar_entries
    DROP COLUMN IF EXISTS ai_generated,
    DROP COLUMN IF EXISTS ai_model,
    DROP COLUMN IF EXISTS hitl_reviewed_by,
    DROP COLUMN IF EXISTS ai_generated_at;

COMMIT;
