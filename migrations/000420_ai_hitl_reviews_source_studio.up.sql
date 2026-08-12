-- 000420_ai_hitl_reviews_source_studio.up.sql
-- Extends the ai_hitl_reviews.source CHECK constraint (migration 000395) to
-- permit 'studio' so the AI Content Studio (§75, ADR-076) can route its
-- drafts into the HITL review queue with source='studio', per the §75
-- Review Routing spec ("All studio drafts enter §22 content_reviews with
-- source = 'studio'").
--
-- Ref: japaconnect-engineering/sections/75-ai-content-studio.md (Review Routing)
-- Ref: japaconnect-engineering/adr/ADR-076-ai-content-studio.md

BEGIN;

ALTER TABLE ai_hitl_reviews
    DROP CONSTRAINT IF EXISTS ai_hitl_reviews_source_check;

ALTER TABLE ai_hitl_reviews
    ADD CONSTRAINT ai_hitl_reviews_source_check
    CHECK (source IN ('manual', 'agent_draft_request', 'agent_tool', 'studio'));

COMMIT;
