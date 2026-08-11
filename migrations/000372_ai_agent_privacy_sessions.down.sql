-- 000372_ai_agent_privacy_sessions.down.sql
-- Reverses 000372_ai_agent_privacy_sessions.up.sql.
-- Order: drop RBAC grants → drop permission nodes → drop settings →
-- drop tool_calls (depends on sessions + ai_hitl_reviews FK) → drop
-- sessions → drop privacy columns.
--
-- Ref: japaconnect-engineering/sections/103-ai-agent-executor.md

BEGIN;

-- Revoke the four new nodes from super_admin and member.
DELETE FROM role_permissions rp
USING permissions p, roles r
WHERE rp.permission_id = p.id
  AND rp.role_id = r.id
  AND p.key IN ('agent.use', 'agent.draft_request', 'agent.external_search', 'content.assign_author')
  AND r.slug IN ('super_admin', 'member');

-- Remove the four permission nodes.
DELETE FROM permissions
WHERE key IN ('agent.use', 'agent.draft_request', 'agent.external_search', 'content.assign_author');

-- Remove the 9 agent.* system_settings.
DELETE FROM system_settings
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND key IN (
    'agent.enabled',
    'agent.max_tool_calls',
    'agent.timeout_seconds',
    'agent.default_persona_id',
    'agent.external_search_provider',
    'agent.external_search_api_key',
    'agent.presearch_full_threshold',
    'agent.cost_limit_per_request_usd',
    'agent.guest_enabled'
  );

-- Drop agent tables (tool_calls first — FK to sessions).
DROP TABLE IF EXISTS ai_agent_tool_calls;
DROP TABLE IF EXISTS ai_agent_sessions;

-- Drop privacy exemption columns.
ALTER TABLE users DROP COLUMN IF EXISTS ai_exempt;
ALTER TABLE custom_field_values DROP COLUMN IF EXISTS ai_exempt;
ALTER TABLE contents DROP COLUMN IF EXISTS ai_exempt;

COMMIT;
