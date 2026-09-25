-- 000449_contents_adr088_ai_columns.up.sql
-- Add ADR-088 universal AI content generation columns to the contents table.
-- All admin-managed content tables in §§79–101 must carry ai_generated,
-- ai_model, hitl_reviewed_by, ai_generated_at, hitl_reviewed_at.
-- The Intelligence System promoters (§INTEL, ADR-112) create AI-generated
-- guide content rows in the contents table; without these columns the
-- AI-generated content is indistinguishable from human-authored content.
--
-- Ref: japaconnect-engineering/adr/ADR-088-destination-wiki.md (universal AI content generation)
--      japaconnect-engineering/audit/intelligence-audit.md (INTEL-003)

BEGIN;

ALTER TABLE contents
    ADD COLUMN IF NOT EXISTS ai_generated      boolean NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS ai_model          text,
    ADD COLUMN IF NOT EXISTS ai_generated_at   timestamptz,
    ADD COLUMN IF NOT EXISTS hitl_reviewed_by  uuid REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS hitl_reviewed_at  timestamptz;

COMMIT;
