-- 000473_cv_format_sessions_adr088_columns.up.sql
-- Add missing ADR-088 columns to cv_format_sessions.
-- CV formatting is user-initiated (not admin-managed editorial content),
-- so hitl_reviewed_* columns are optional and will remain NULL for most
-- sessions. They are added for audit/provenance consistency per ADR-088.

BEGIN;

ALTER TABLE cv_format_sessions
    ADD COLUMN IF NOT EXISTS ai_generated boolean NOT NULL DEFAULT true,
    ADD COLUMN IF NOT EXISTS ai_prompt_hash text,
    ADD COLUMN IF NOT EXISTS hitl_reviewed_by uuid REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS hitl_reviewed_at timestamptz,
    ADD COLUMN IF NOT EXISTS hitl_review_notes text;

COMMIT;
