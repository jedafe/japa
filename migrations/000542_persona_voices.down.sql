-- 000542_persona_voices.down.sql
-- Reverts persona_voices table and voice_id column.
-- Note: this does NOT restore per-persona voice_prompt values if they were
-- migrated to shared voices; a data-only rollback would need a backup.

BEGIN;

ALTER TABLE ai_personas DROP COLUMN IF EXISTS voice_id;

DROP TABLE IF EXISTS persona_voices;

COMMIT;
