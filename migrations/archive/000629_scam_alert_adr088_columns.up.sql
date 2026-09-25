-- 000629_scam_alert_adr088_columns.up.sql
-- Adds the ADR-088 universal AI-generation columns to scam_reports.
-- scam_report is a registered agent tool type (agent_tools_registry.go), so
-- AI-drafted reports can exist and require the standard HITL gate.
--   ai_generated       BOOLEAN NOT NULL DEFAULT FALSE — was this report AI-generated?
--   ai_model           TEXT                           — model identifier (e.g. gpt-4o)
--   ai_generated_at    TIMESTAMPTZ                    — when the AI generated the draft
--   hitl_reviewed_by   UUID REFERENCES users(id)      — the human reviewer (distinct
--                       from reviewed_by, which records the moderation decision)
--   hitl_reviewed_at   TIMESTAMPTZ                    — when the HITL review occurred
-- All columns are additive/nullable-with-default — zero downtime.
--
-- Ref: japaconnect-engineering/sections/81-scam-alert-board.md
-- Ref: japaconnect-engineering/adr/ADR-088 (universal AI content generation)
-- Pattern: 000457_calendar_adr088_ai_columns.up.sql, 000506_calendar_adr088_remaining_columns.up.sql

BEGIN;

ALTER TABLE scam_reports
    ADD COLUMN IF NOT EXISTS ai_generated      BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS ai_model          TEXT,
    ADD COLUMN IF NOT EXISTS ai_generated_at   TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS hitl_reviewed_by  UUID REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS hitl_reviewed_at  TIMESTAMPTZ;

COMMIT;
