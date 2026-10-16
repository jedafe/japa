-- 000878_agent_spec_search_tool.down.sql

BEGIN;

DELETE FROM ai_tools WHERE name = 'spec.search';

COMMIT;
