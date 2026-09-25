-- 000391_fix_community_locator_adr088_conformance.up.sql
-- Fix BUG-001: Add ADR-088 universal AI content generation columns to
-- community_organisations. All admin-managed content tables in §§79–101
-- must carry ai_generated, ai_model, ai_prompt_hash, hitl_reviewed_by,
-- hitl_reviewed_at, hitl_review_notes.
-- Fix BUG-002: is_japabot_indexed should be a GENERATED ALWAYS AS column
-- per §101 spec (status = 'published' AND is_active = TRUE) STORED.
-- The original migration (000343) created it as a plain boolean default false.
--
-- Ref: japaconnect-engineering/sections/101-diaspora-community-locator.md
--      japaconnect-engineering/adr/ADR-088-destination-wiki.md (universal AI content generation)
--      japaconnect-engineering/adr/ADR-109-community-locator.md

BEGIN;

-- BUG-001: Add ADR-088 AI generation columns.
ALTER TABLE community_organisations
    ADD COLUMN IF NOT EXISTS ai_generated      boolean NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS ai_model          text,
    ADD COLUMN IF NOT EXISTS ai_prompt_hash    text,
    ADD COLUMN IF NOT EXISTS hitl_reviewed_by  uuid REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS hitl_reviewed_at  timestamptz,
    ADD COLUMN IF NOT EXISTS hitl_review_notes text;

-- BUG-002: Convert is_japabot_indexed from plain boolean to GENERATED ALWAYS AS.
-- Must drop the plain column and re-add as GENERATED STORED.
-- Existing values are recomputed from status + is_active.
ALTER TABLE community_organisations DROP COLUMN IF EXISTS is_japabot_indexed;
ALTER TABLE community_organisations ADD COLUMN is_japabot_indexed BOOLEAN GENERATED ALWAYS AS (
    status = 'published' AND is_active = TRUE
) STORED;

COMMIT;
