-- 000411_content_reviews_source.up.sql
-- Participation Orchestration (§76, ADR-077 §4): add a `source` column to
-- content_reviews so persona-action-sourced reviews can be filtered from
-- editorial reviews in the admin queue. The §76 spec requires
-- content_reviews.source = 'persona_action' for queued persona actions;
-- previously the column was added only to ai_hitl_reviews (000395), which
-- left content_reviews unable to distinguish persona actions.
--
-- Values:
--   'editorial'         — default; existing editorial-review rows (§22)
--   'persona_action'    — queued persona action awaiting admin approval (§76)
--
-- Ref: japaconnect-engineering/sections/76-participation-orchestration.md
--      japaconnect-engineering/adr/ADR-077-participation-orchestration-engine.md (§4)

BEGIN;

ALTER TABLE content_reviews
    ADD COLUMN IF NOT EXISTS source text NOT NULL DEFAULT 'editorial'
    CHECK (source IN ('editorial', 'persona_action'));

CREATE INDEX IF NOT EXISTS idx_content_reviews_source
    ON content_reviews (tenant_id, source, status, created_at DESC);

COMMIT;
