-- 000524_seed_intelligence_agent_tools.down.sql

BEGIN;

DELETE FROM ai_tools
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND name IN ('source.crawl_now', 'record.update_now', 'content.regenerate_from_source');

DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions
    WHERE key IN ('agent.source.crawl_now', 'agent.record.update_now', 'content.regenerate')
);

DELETE FROM permissions
WHERE key IN ('agent.source.crawl_now', 'agent.record.update_now', 'content.regenerate');

COMMIT;
