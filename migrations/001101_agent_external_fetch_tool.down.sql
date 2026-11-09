-- 001101_agent_external_fetch_tool.down.sql
--
-- Removes the external.fetch catalog row, its tunables, and the
-- agent.external_fetch permission node (+ role grants).

BEGIN;

DELETE FROM ai_tools WHERE name = 'external.fetch';

DELETE FROM system_settings
WHERE key IN (
    'agent.external_fetch_timeout_seconds',
    'agent.external_fetch_max_bytes',
    'agent.external_fetch_max_chars',
    'agent.external_fetch_cache_ttl_seconds'
);

DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE key = 'agent.external_fetch');

DELETE FROM permissions WHERE key = 'agent.external_fetch';

COMMIT;
