-- 000506_calendar_adr088_remaining_columns.down.sql
-- Reverses 000506: removes the 3 remaining ADR-088 columns.
-- Ref: japaconnect-engineering/skills/geo-aware-tool-pattern.md

BEGIN;

ALTER TABLE calendar_entries
    DROP COLUMN IF EXISTS hitl_review_notes,
    DROP COLUMN IF EXISTS hitl_reviewed_at,
    DROP COLUMN IF EXISTS ai_prompt_hash;

COMMIT;
