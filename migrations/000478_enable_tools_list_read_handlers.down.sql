-- 000478_enable_tools_list_read_handlers.down.sql
BEGIN;

UPDATE ai_tools
SET is_enabled = false
WHERE name IN ('tools.list', 'tools.read');

COMMIT;
