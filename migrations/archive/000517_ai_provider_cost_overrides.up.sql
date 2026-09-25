-- 000517_ai_provider_cost_overrides.up.sql
-- Section 20 (AI Core) — Provider/model cost override JSON map.
--
-- Seeds the empty JSON object for the Control Panel key-value editor.
--
-- Ref: japaconnect-engineering/conventions/system-settings-catalog.md

BEGIN;

INSERT INTO system_settings (tenant_id, key, value) VALUES
    ('00000000-0000-0000-0000-000000000001', 'ai.provider_cost_overrides', '{}')
ON CONFLICT (tenant_id, key) DO NOTHING;

COMMIT;
