-- 000391_fix_community_locator_adr088_conformance.down.sql
-- Revert ADR-088 AI generation columns and is_japabot_indexed GENERATED column.
-- Restores the original 000343 schema (plain boolean is_japabot_indexed).

BEGIN;

-- Revert BUG-002: restore is_japabot_indexed as plain boolean default false.
ALTER TABLE community_organisations DROP COLUMN IF EXISTS is_japabot_indexed;
ALTER TABLE community_organisations ADD COLUMN is_japabot_indexed boolean NOT NULL DEFAULT false;

-- Revert BUG-001: drop ADR-088 AI generation columns.
ALTER TABLE community_organisations
    DROP COLUMN IF EXISTS ai_generated,
    DROP COLUMN IF EXISTS ai_model,
    DROP COLUMN IF EXISTS ai_prompt_hash,
    DROP COLUMN IF EXISTS hitl_reviewed_by,
    DROP COLUMN IF EXISTS hitl_reviewed_at,
    DROP COLUMN IF EXISTS hitl_review_notes;

COMMIT;
