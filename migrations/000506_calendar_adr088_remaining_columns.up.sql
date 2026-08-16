-- 000506_calendar_adr088_remaining_columns.up.sql
-- ADR-088 universal AI content generation: add the remaining 3 standard
-- columns to calendar_entries that were missing from migration 000457.
--   ai_prompt_hash     TEXT          — SHA-256 of the prompt (audit reproducibility)
--   hitl_reviewed_at   TIMESTAMPTZ   — when the HITL review occurred
--   hitl_review_notes  TEXT          — reviewer notes
--
-- Ref: japaconnect-engineering/skills/geo-aware-tool-pattern.md (ADR-088 columns)
-- Ref: japaconnect-engineering/sections/86-community-calendar.md

BEGIN;

ALTER TABLE calendar_entries
    ADD COLUMN ai_prompt_hash    TEXT,
    ADD COLUMN hitl_reviewed_at  TIMESTAMPTZ,
    ADD COLUMN hitl_review_notes TEXT;

COMMIT;
