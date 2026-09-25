-- 000477_ai_hitl_reviews_target_table.up.sql
-- Extends ai_hitl_reviews to support HITL review of tool table drafts
-- (housing_guides, visa_routes, attestation_pathways, etc.) in addition
-- to contents rows. The existing nullable content_id column is reused
-- for the target record's UUID; target_table identifies which table the
-- record lives in.
--
-- Ref: ADR-076 Amendment 1 (Studio–Intelligence integration, Approach B)
-- Ref: ADR-088 (universal AI content generation pattern)

BEGIN;

ALTER TABLE ai_hitl_reviews
    ADD COLUMN IF NOT EXISTS target_table text NOT NULL DEFAULT 'contents';

-- Index for filtering reviews by target table (admin review queue).
CREATE INDEX IF NOT EXISTS idx_ai_hitl_reviews_target_table
    ON ai_hitl_reviews (tenant_id, target_table, status);

COMMIT;
