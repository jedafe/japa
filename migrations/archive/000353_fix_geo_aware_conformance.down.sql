-- 000353_fix_geo_aware_conformance.down.sql
-- Rollback: remove geo-aware conformance fix columns.

BEGIN;

ALTER TABLE language_test_requirements
    DROP COLUMN IF EXISTS is_japabot_indexed;

ALTER TABLE story_meta
    DROP COLUMN IF EXISTS ai_prompt_hash,
    DROP COLUMN IF EXISTS hitl_review_notes;

COMMIT;
