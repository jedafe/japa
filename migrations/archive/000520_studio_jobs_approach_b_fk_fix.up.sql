-- 000520_studio_jobs_approach_b_fk_fix.up.sql
-- ADR-076 Amendment 1 (Approach B): studio_jobs now tracks drafts for both
-- contents rows and dedicated tool tables (housing_guides, visa_routes, etc.).
-- The existing FKs on draft_content_id (contents) and review_id (content_reviews)
-- were written before the tool-table HITL flow (ai_hitl_reviews with target_table
-- from migration 000477). Drop the FKs so the columns can store tool-table row
-- IDs and ai_hitl_reviews IDs respectively. Referential integrity for contents
-- is still enforced by the content publisher and the tool-table publisher.
--
-- Ref: japaconnect-engineering/adr/ADR-076-ai-content-studio.md
-- Ref: japaconnect-engineering/sections/75-ai-content-studio.md

BEGIN;

ALTER TABLE studio_jobs
    DROP CONSTRAINT IF EXISTS studio_jobs_draft_content_id_fkey,
    DROP CONSTRAINT IF EXISTS studio_jobs_review_id_fkey;

COMMIT;
