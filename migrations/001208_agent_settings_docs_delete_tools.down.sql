-- 001208_agent_settings_docs_delete_tools.down.sql
-- Removes the docs.delete / settings.get / settings.set agent tool rows.

BEGIN;

DELETE FROM ai_tools
WHERE name IN ('docs.delete', 'settings.get', 'settings.set');

COMMIT;
