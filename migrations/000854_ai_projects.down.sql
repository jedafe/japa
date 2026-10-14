-- Down: drop AI Projects table + membership columns/indexes.
BEGIN;

DROP INDEX IF EXISTS idx_ai_agent_sessions_project;
ALTER TABLE ai_agent_sessions DROP COLUMN IF EXISTS project_id;

DROP INDEX IF EXISTS idx_ai_conversations_project;
ALTER TABLE ai_conversations DROP COLUMN IF EXISTS project_id;

DROP TABLE IF EXISTS ai_projects;

COMMIT;
