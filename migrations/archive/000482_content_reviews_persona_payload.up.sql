-- 000482_content_reviews_persona_payload.up.sql
-- Participation Orchestration (§76, ADR-077 §4): add a dedicated `persona_payload`
-- jsonb column to content_reviews for storing the actionPayload JSON that
-- DispatchAction reads to route approved persona actions to the correct existing
-- endpoint (ADR-077 §5).
--
-- Previously the action payload was stored in `content_reviews.feedback` (a `text`
-- column shared with the editorial review flow which uses it as free-text). Storing
-- JSON in a text column works but is fragile: no JSON validation at the DB level,
-- no indexing, and a future migration changing the editorial flow could break JSON
-- parsing. This migration adds a typed `jsonb` column populated only for
-- `source='persona_action'` rows; `feedback` remains for editorial free-text.
--
-- Backfill: copy any existing persona_action rows' feedback JSON into persona_payload
-- so in-flight queued actions are not lost.
--
-- Ref: japaconnect-engineering/sections/76-participation-orchestration.md
--      japaconnect-engineering/adr/ADR-077-participation-orchestration-engine.md (§4)

BEGIN;

ALTER TABLE content_reviews
    ADD COLUMN IF NOT EXISTS persona_payload jsonb;

-- Backfill existing persona_action rows from the text feedback column so
-- in-flight queued actions survive the column split.
UPDATE content_reviews
SET persona_payload = feedback::jsonb
WHERE source = 'persona_action'
  AND feedback IS NOT NULL
  AND feedback <> ''
  AND persona_payload IS NULL;

-- Index for fast persona-action queue lookups (§76 admin queue endpoint).
CREATE INDEX IF NOT EXISTS idx_content_reviews_persona_payload
    ON content_reviews (tenant_id, source, status, created_at DESC)
    WHERE source = 'persona_action';

COMMIT;
