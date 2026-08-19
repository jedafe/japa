-- 000523_seed_agent_regenerate_content_on_promote.up.sql
-- Gating setting for promoter-triggered content regeneration from
-- intelligence records (§75, ADR-076 Amendment 2). Default false until the
-- on-demand intelligence-to-content flow is fully verified.
--
-- Ref: docs/superpowers/specs/2026-08-19-on-demand-intel-to-content.md

BEGIN;

INSERT INTO system_settings (tenant_id, key, value, is_public)
VALUES ('00000000-0000-0000-0000-000000000001', 'agent.regenerate_content_on_promote', '"false"', false)
ON CONFLICT (tenant_id, key) DO NOTHING;

COMMIT;
