-- 000523_seed_agent_regenerate_content_on_promote.down.sql

BEGIN;

DELETE FROM system_settings
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND key = 'agent.regenerate_content_on_promote';

COMMIT;
