-- 000353_fix_geo_aware_conformance.up.sql
-- Fixes geo-aware conformance gaps found in the 2026-08-09 audit:
--
-- 1. language_test_requirements: add is_japabot_indexed (ADR-087)
-- 2. story_meta: add ai_prompt_hash and hitl_review_notes (ADR-088)
--
-- Ref: .jeos/skills/geo-aware-tool-pattern.md
--      .jeos/adr/ADR-087 (JapaBot RAG context scope)
--      .jeos/adr/ADR-088 (Universal AI content generation)

BEGIN;

-- =====================================================================
-- 1. language_test_requirements: add is_japabot_indexed (ADR-087)
--    Required per geo-aware-tool-pattern.md list of tables that need
--    JapaBot RAG indexability.
-- =====================================================================
ALTER TABLE language_test_requirements
    ADD COLUMN IF NOT EXISTS is_japabot_indexed boolean NOT NULL DEFAULT false;

-- Backfill: mark existing published+active rows as indexable.
UPDATE language_test_requirements
SET is_japabot_indexed = true
WHERE status = 'published' AND is_active = true;

-- =====================================================================
-- 2. story_meta: add missing ADR-088 columns
--    ai_prompt_hash (SHA-256 of prompt for audit reproducibility)
--    hitl_review_notes (admin notes during HITL review)
-- =====================================================================
ALTER TABLE story_meta
    ADD COLUMN IF NOT EXISTS ai_prompt_hash   text,
    ADD COLUMN IF NOT EXISTS hitl_review_notes text;

COMMIT;
