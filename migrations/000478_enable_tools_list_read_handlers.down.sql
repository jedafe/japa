-- 000478_enable_tools_list_read_handlers.down.sql
BEGIN;

UPDATE ai_agent_tools
SET is_enabled = false
WHERE function_name IN ('tools.list', 'tools.read');

COMMIT;
