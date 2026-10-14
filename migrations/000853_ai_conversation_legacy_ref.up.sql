-- 000853_ai_conversation_legacy_ref.up.sql
-- ADR-130 Phase 4: Honcho → ai_conversations backfill marker.
--
-- ai_conversations.legacy_ref tags a thread that was migrated from a
-- legacy store so the backfill is idempotent and the legacy list union
-- can exclude migrated threads:
--
--   'honcho:{conversation_id}' — one row per Honcho
--   metadata.conversation_id group (or entry id for ungrouped entries).
--
-- The partial unique index on (user_id, legacy_ref) makes a re-run of the
-- ai:migrate_conversations task a no-op instead of duplicating threads.
--
-- Ref: japaconnect-engineering/adr/ADR-130-unified-japa-ai.md

BEGIN;

ALTER TABLE ai_conversations
    ADD COLUMN legacy_ref text NOT NULL DEFAULT '';

CREATE UNIQUE INDEX idx_ai_conversations_legacy_ref
    ON ai_conversations (user_id, legacy_ref)
    WHERE legacy_ref <> '';

COMMIT;
