-- 000515_ai_cost_and_cache_settings.down.sql
-- Reverts 000515 by removing the seeded AI cost and cache settings.

BEGIN;

DELETE FROM system_settings
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND key IN (
    'ai.cost_per_1k_tokens_usd',
    'ai.cache_enabled',
    'ai.cache_ttl_seconds',
    'agent.cost_per_1k_tokens'
  );

COMMIT;
