-- 000420_ai_hitl_reviews_source_studio.down.sql
-- Reverses 000420: restores the original ai_hitl_reviews.source CHECK
-- constraint (migration 000395) that excludes 'studio'. Any rows with
-- source='studio' must be reclassified to 'manual' before this runs.

BEGIN;

ALTER TABLE ai_hitl_reviews
    DROP CONSTRAINT IF EXISTS ai_hitl_reviews_source_check;

ALTER TABLE ai_hitl_reviews
    ADD CONSTRAINT ai_hitl_reviews_source_check
    CHECK (source IN ('manual', 'agent_draft_request', 'agent_tool'));

COMMIT;
