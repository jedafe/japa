-- 000457_calendar_adr088_ai_columns.up.sql
-- ADR-088 universal AI content generation: add the standard AI generation
-- columns to calendar_entries so admin-managed calendar content can be
-- AI-generated via the §20 AI Gateway with mandatory HITL review before
-- publish.
--   ai_generated       BOOLEAN DEFAULT FALSE — was this entry AI-generated?
--   ai_model           TEXT                  — model identifier (e.g. gpt-4o)
--   hitl_reviewed_by   UUID REFERENCES users(id) — human reviewer (distinct
--                                                  from reviewed_by which
--                                                  stamps the publish action)
--   ai_generated_at    TIMESTAMPTZ           — when the AI generated the draft
--
-- Ref: japaconnect-engineering/adr/ADR-088-destination-wiki.md (universal AI
--      content generation pattern)
-- Ref: japaconnect-engineering/sections/86-community-calendar.md

BEGIN;

ALTER TABLE calendar_entries
    ADD COLUMN IF NOT EXISTS ai_generated     BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS ai_model         TEXT,
    ADD COLUMN IF NOT EXISTS hitl_reviewed_by UUID REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS ai_generated_at  TIMESTAMPTZ;

COMMIT;
