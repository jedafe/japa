-- 000493_seed_agent_temperature.down.sql
-- Reverts the agent.temperature system_setting seed.

BEGIN;

DELETE FROM system_settings
 WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
   AND key = 'agent.temperature';

COMMIT;
