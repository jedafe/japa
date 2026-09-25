-- 000408_language_test_req_is_japabot_indexed.up.sql
-- Fix BUG-001: is_japabot_indexed missing from language_test_requirements.
-- Per ADR-088 JapaBot Indexability Pattern, all admin-managed content tables
-- for §§79–101 must carry is_japabot_indexed as a GENERATED ALWAYS AS column.
-- Language test requirements are RAG-retrievable (JapaBot answers "what IELTS
-- score for Japan skilled worker visa?"). Peer features (housing_guides 000407,
-- attestation_pathways 000375, story_meta 000298, community_organisations 000391)
-- all have this column.
--
-- The generation expression requires HITL review (hitl_reviewed_by IS NOT NULL)
-- in addition to published + active, because ADR-088 mandates HITL review before
-- publish for AI-generated content. Human-admin-created requirements that skip
-- the approve endpoint still have hitl_reviewed_by NULL — they are indexed only
-- if explicitly approved.
--
-- Ref: japaconnect-engineering/sections/99-language-test-hub.md
--      japaconnect-engineering/adr/ADR-088-destination-wiki.md (universal AI content generation)

BEGIN;

ALTER TABLE language_test_requirements ADD COLUMN IF NOT EXISTS is_japabot_indexed BOOLEAN GENERATED ALWAYS AS (
    status = 'published' AND is_active = TRUE AND hitl_reviewed_by IS NOT NULL
) STORED;

COMMIT;
