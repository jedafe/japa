-- 000524_seed_intelligence_agent_tools.up.sql
-- Section 103 (AI Agent Executor) — On-demand intelligence agent tools.
--
-- Seeds the source/record/content agent tools that enqueue background
-- workers for the on-demand intelligence-to-content pipeline, plus the
-- RBAC permission nodes and role grants those tools are gated by.
--
-- Ref: docs/superpowers/specs/2026-08-19-on-demand-intel-to-content.md
-- Ref: japaconnect-engineering/sections/103-ai-agent-executor.md

BEGIN;

-- 1. Permission nodes
INSERT INTO permissions (key, description) VALUES
    ('agent.source.crawl_now',     'AI Agent — trigger an immediate crawl of an intelligence source'),
    ('agent.record.update_now',    'AI Agent — resolve a record or source and enqueue a full refresh'),
    ('content.regenerate',         'AI Agent — regenerate an existing content row from an intelligence record')
ON CONFLICT (key) DO NOTHING;

-- 2. Grant to admin and super_admin on the default tenant
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('admin', 'super_admin')
  AND p.key IN ('agent.source.crawl_now', 'agent.record.update_now', 'content.regenerate')
ON CONFLICT DO NOTHING;

-- 3. Tool catalog rows
INSERT INTO ai_tools (tenant_id, name, description, category, config, is_enabled, required_permission) VALUES
('00000000-0000-0000-0000-000000000001', 'source.crawl_now',
 'Enqueue an immediate crawl for a single enabled intelligence source. The crawl runs asynchronously through the intel:fetch_source worker.',
 'agent_intelligence',
 '{
    "function_name": "source.crawl_now",
    "description": "Enqueue an immediate crawl for a single enabled intelligence source.",
    "parameters": {
        "type": "object",
        "properties": {
            "source_id": { "type": "string", "description": "UUID of the intel source to crawl" }
        },
        "required": ["source_id"]
    },
    "handler": "SourceCrawlNow",
    "reads_user_data": false,
    "writes_user_data": false,
    "requires_hitl": false,
    "is_blocked": false
}',
 true, 'agent.source.crawl_now'),

('00000000-0000-0000-0000-000000000001', 'record.update_now',
 'Resolve an intel record or source and enqueue the full intelligence fetch/change-detection/extraction/promotion chain. If record_id is provided, the source is read from the record; otherwise source_id is used directly.',
 'agent_intelligence',
 '{
    "function_name": "record.update_now",
    "description": "Resolve an intel record or source and enqueue the full update chain.",
    "parameters": {
        "type": "object",
        "properties": {
            "record_id": { "type": "string", "description": "Optional UUID of the intel record to refresh" },
            "source_id": { "type": "string", "description": "Optional UUID of the source to crawl directly" }
        },
        "required": []
    },
    "handler": "RecordUpdateNow",
    "reads_user_data": false,
    "writes_user_data": false,
    "requires_hitl": false,
    "is_blocked": false
}',
 true, 'agent.record.update_now'),

('00000000-0000-0000-0000-000000000001', 'content.regenerate_from_source',
 'Enqueue the content:regenerate_from_source worker to rewrite an existing contents row from a linked intelligence record extraction. Respects HITL, high-stakes, and manual-edit guards.',
 'agent_intelligence',
 '{
    "function_name": "content.regenerate_from_source",
    "description": "Rewrite an existing contents row from a linked intelligence record extraction.",
    "parameters": {
        "type": "object",
        "properties": {
            "content_id": { "type": "string", "description": "UUID of the contents row to regenerate" },
            "intel_record_id": { "type": "string", "description": "UUID of the intelligence record whose extraction supplies the facts" }
        },
        "required": ["content_id", "intel_record_id"]
    },
    "handler": "ContentRegenerateFromSource",
    "reads_user_data": false,
    "writes_user_data": true,
    "requires_hitl": false,
    "is_blocked": false
}',
 true, 'content.regenerate');

COMMIT;
