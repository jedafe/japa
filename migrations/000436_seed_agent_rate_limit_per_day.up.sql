-- 000436_seed_agent_rate_limit_per_day.up.sql
-- Section 103 (AI Agent Executor) — seeds the missing agent.rate_limit_per_day
-- system_setting key.
--
-- The setting is read in internal/ai/agent.go (checkRateLimit) with a default
-- of 50, but was omitted from migration 000372's system_settings INSERT and
-- from the system-settings-catalog.md convention file. Without this seed,
-- admins cannot configure the agent daily rate limit via the admin UI — it
-- always falls back to the hardcoded default.
--
-- Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (line 605:
--   "Rate limited: agent.rate_limit_per_day setting")
-- Ref: japaconnect-engineering/roadmaps/ROADMAP-AI-AGENT.md (line 185)
--
-- Idempotent; safe to re-run.

BEGIN;

INSERT INTO system_settings (tenant_id, key, value) VALUES
    ('00000000-0000-0000-0000-000000000001', 'agent.rate_limit_per_day', '50')
ON CONFLICT (tenant_id, key) DO NOTHING;

COMMIT;
