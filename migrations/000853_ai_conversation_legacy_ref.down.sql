-- Down: drop the Honcho backfill marker + its idempotency index.
BEGIN;

DROP INDEX IF EXISTS idx_ai_conversations_legacy_ref;
ALTER TABLE ai_conversations DROP COLUMN IF EXISTS legacy_ref;

COMMIT;
