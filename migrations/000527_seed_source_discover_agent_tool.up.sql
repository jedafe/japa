-- 000527_seed_source_discover_agent_tool.up.sql
-- Section 103 (AI Agent Executor) — source.discover tool.
--
-- Adds the source.discover agent tool and the agent.auto_enable_discovered_sources
-- setting. The tool creates new intel_sources with status='proposed' and
-- enabled=false; admins can review and enable them.
--
-- Ref: docs/superpowers/specs/2026-08-19-on-demand-intel-to-content.md

BEGIN;

INSERT INTO permissions (key, description) VALUES
    ('agent.source.discover', 'AI Agent — discover and propose new intelligence sources')
ON CONFLICT (key) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('admin', 'super_admin')
  AND p.key = 'agent.source.discover'
ON CONFLICT DO NOTHING;

INSERT INTO ai_tools (tenant_id, name, description, category, config, is_enabled, required_permission) VALUES
('00000000-0000-0000-0000-000000000001', 'source.discover',
 'Search the web for candidate intelligence sources and create them with status=proposed, enabled=false for admin review. Optionally auto-enables high-trust sources when agent.auto_enable_discovered_sources is true.',
 'agent_intelligence',
 '{
    "function_name": "source.discover",
    "description": "Search the web for candidate intelligence sources and propose them for admin review.",
    "parameters": {
        "type": "object",
        "properties": {
            "query": { "type": "string", "description": "Search query describing the kind of source to find (e.g. \"scholarships in Nigeria 2026\")" },
            "category": { "type": "string", "description": "Optional category for the discovered source (e.g. scholarship, job, guide)" },
            "country": { "type": "string", "description": "Optional country filter for the search" },
            "limit": { "type": "integer", "description": "Max candidate results (default 5, max 20)" }
        },
        "required": ["query"]
    },
    "handler": "SourceDiscover",
    "reads_user_data": false,
    "writes_user_data": true,
    "requires_hitl": false,
    "is_blocked": false
}',
 true, 'agent.source.discover');

INSERT INTO system_settings (tenant_id, key, value) VALUES
    ('00000000-0000-0000-0000-000000000001', 'agent.auto_enable_discovered_sources', 'false'::jsonb)
ON CONFLICT (tenant_id, key) DO NOTHING;

COMMIT;
