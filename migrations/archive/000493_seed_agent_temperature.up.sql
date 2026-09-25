-- 000493_seed_agent_temperature.up.sql
-- Section 103 (AI Agent Executor) — seeds the agent.temperature
-- system_setting key.
--
-- The agent loop in internal/ai/agent.go previously hardcoded
-- Temperature: 0.3 in the ChatWithTools call. Per Rule 3, configurable
-- values must come from system_settings, not hardcoded constants.
-- This migration seeds the setting so admins can tune the LLM
-- sampling temperature per tenant via the admin UI.
--
-- Ref: japaconnect-engineering/sections/103-ai-agent-executor.md
-- Ref: japaconnect-engineering/AGENTS.md (Rule 3)
--
-- Idempotent; safe to re-run.

BEGIN;

INSERT INTO system_settings (tenant_id, key, value) VALUES
    ('00000000-0000-0000-0000-000000000001', 'agent.temperature', '0.3')
ON CONFLICT (tenant_id, key) DO NOTHING;

COMMIT;
