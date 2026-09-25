-- 000527_seed_source_discover_agent_tool.down.sql

BEGIN;

DELETE FROM ai_tools
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND name = 'source.discover';

DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE key = 'agent.source.discover'
);

DELETE FROM permissions
WHERE key = 'agent.source.discover';

DELETE FROM system_settings
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND key = 'agent.auto_enable_discovered_sources';

COMMIT;
