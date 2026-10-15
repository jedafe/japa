-- 000864_agent_comparison_list_tool.down.sql

BEGIN;

DELETE FROM ai_tools WHERE name = 'comparison.list';

COMMIT;
