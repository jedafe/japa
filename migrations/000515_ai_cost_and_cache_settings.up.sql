-- 000515_ai_cost_and_cache_settings.up.sql
-- Section 20 (AI Core) — Cost monitoring + semantic cache settings.
--
-- Seeds the system_settings keys used by:
--   - internal/ai/cost.go (per-provider, per-model, and default AI pricing)
--   - internal/ai/cache.go (semantic cache toggle + TTL)
--   - internal/ai/agent.go (agent.cost_per_1k_tokens fallback)
--
-- Ref: japaconnect-engineering/conventions/system-settings-catalog.md
--      japaconnect-engineering/conventions/valkey-keys-catalog.md

BEGIN;

INSERT INTO system_settings (tenant_id, key, value) VALUES
    ('00000000-0000-0000-0000-000000000001', 'ai.cost_per_1k_tokens_usd', '0.002'),
    ('00000000-0000-0000-0000-000000000001', 'ai.cache_enabled',          'false'),
    ('00000000-0000-0000-0000-000000000001', 'ai.cache_ttl_seconds',      '300'),
    ('00000000-0000-0000-0000-000000000001', 'agent.cost_per_1k_tokens',  '0.002')
ON CONFLICT (tenant_id, key) DO NOTHING;

COMMIT;
