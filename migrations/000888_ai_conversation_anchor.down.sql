DROP INDEX IF EXISTS idx_ai_conversations_anchor;
ALTER TABLE ai_conversations DROP COLUMN IF EXISTS anchor;
