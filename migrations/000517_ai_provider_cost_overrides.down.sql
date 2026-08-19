-- 000517_ai_provider_cost_overrides.down.sql
-- Remove the AI provider cost overrides setting.

BEGIN;

DELETE FROM system_settings WHERE key = 'ai.provider_cost_overrides';

COMMIT;
