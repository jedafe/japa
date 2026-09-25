-- 000436_seed_agent_rate_limit_per_day.down.sql
-- Reverts the agent.rate_limit_per_day system_setting seed.

BEGIN;

DELETE FROM system_settings
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND key = 'agent.rate_limit_per_day';

COMMIT;
