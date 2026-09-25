-- 000629_scam_alert_adr088_columns.down.sql
-- Reverses 000629: drops the ADR-088 AI-generation columns from scam_reports.

BEGIN;

ALTER TABLE scam_reports
    DROP COLUMN IF EXISTS ai_generated,
    DROP COLUMN IF EXISTS ai_model,
    DROP COLUMN IF EXISTS ai_generated_at,
    DROP COLUMN IF EXISTS hitl_reviewed_by,
    DROP COLUMN IF EXISTS hitl_reviewed_at;

COMMIT;
