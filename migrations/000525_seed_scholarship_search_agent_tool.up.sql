-- 000525_seed_scholarship_search_agent_tool.up.sql
-- Section 103 (AI Agent Executor) — scholarship.search tool.
--
-- Adds the scholarship.search agent tool and its RBAC permission node.
--
-- Ref: docs/superpowers/specs/2026-08-19-on-demand-intel-to-content.md
-- Ref: japaconnect-engineering/sections/73-scholarship-finder.md

BEGIN;

INSERT INTO permissions (key, description) VALUES
    ('agent.scholarship.search', 'AI Agent — search the scholarships projection table')
ON CONFLICT (key) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('admin', 'super_admin')
  AND p.key = 'agent.scholarship.search'
ON CONFLICT DO NOTHING;

INSERT INTO ai_tools (tenant_id, name, description, category, config, is_enabled, required_permission) VALUES
('00000000-0000-0000-0000-000000000001', 'scholarship.search',
 'Search the scholarships projection table. Returns scholarship records with id, guide_id, intel_record_id, title, slug, country, level of study, award amount, deadline, application URL, and institution. Use this to find scholarships before refreshing their sources or regenerating content.',
 'agent_intelligence',
 '{
    "function_name": "scholarship.search",
    "description": "Search the scholarships projection table for matching records.",
    "parameters": {
        "type": "object",
        "properties": {
            "country": { "type": "string", "description": "Filter by country code or name (e.g. NGA)" },
            "eligible_countries": { "type": "string", "description": "Filter by additional eligible countries" },
            "deadline_after": { "type": "string", "description": "Only return scholarships with deadlines on or after this date (YYYY-MM-DD)" },
            "funding_type": { "type": "string", "description": "Filter by funding type keywords" },
            "level": { "type": "string", "description": "Filter by level of study (e.g. undergraduate, masters, phd)" },
            "limit": { "type": "integer", "description": "Max results (default 20, max 50)" }
        },
        "required": []
    },
    "handler": "ScholarshipSearch",
    "reads_user_data": false,
    "writes_user_data": false,
    "requires_hitl": false,
    "is_blocked": false
}',
 true, 'agent.scholarship.search');

COMMIT;
