-- 000525_seed_scholarship_search_agent_tool.down.sql

BEGIN;

DELETE FROM ai_tools
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND name = 'scholarship.search';

DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE key = 'agent.scholarship.search'
);

DELETE FROM permissions
WHERE key = 'agent.scholarship.search';

COMMIT;
