-- 000842_intel_agent_read_tools.up.sql
--
-- Read-only intelligence tools for the AI agent catalog (§103). The
-- enqueue-only tools (source.crawl_now, record.update_now) return "queued"
-- and walk away; these reads let the agent — and MCP callers on the same
-- executor (§119) — inspect the source and extracted records that resulted.
--
-- RBAC: reuses existing nodes. intel.source.get gates on intel.sources.view
-- (the control-panel registry read node); intel.record.get/list gate on
-- intel.review.view (the node that already exposes AI extractions + evidence
-- in the review UI). No new permission keys are minted.

BEGIN;

INSERT INTO ai_tools (name, description, category, config, is_enabled, required_permission) VALUES
('intel.source.get',
 'Read a single intelligence source registry row: status, enabled flag, trust level, crawl method/frequency, and crawl health (last success/error, consecutive failures).',
 'agent_intelligence',
 '{
    "function_name": "intel.source.get",
    "description": "Read a single intelligence source registry row including status and crawl health.",
    "parameters": {
        "type": "object",
        "properties": {
            "source_id": { "type": "string", "description": "UUID of the intel source to read" }
        },
        "required": ["source_id"]
    },
    "handler": "IntelSourceGet",
    "reads_user_data": false,
    "writes_user_data": false,
    "requires_hitl": false,
    "is_blocked": false
}',
 true, 'intel.sources.view'),

('intel.record.get',
 'Read a single intelligence record including the full extracted payload, per-field confidence map, and evidence. Use after crawl/update to inspect what extraction produced.',
 'agent_intelligence',
 '{
    "function_name": "intel.record.get",
    "description": "Read a single intelligence record including its extracted payload, confidence, and evidence.",
    "parameters": {
        "type": "object",
        "properties": {
            "record_id": { "type": "string", "description": "UUID of the intel record to read" }
        },
        "required": ["record_id"]
    },
    "handler": "IntelRecordGet",
    "reads_user_data": false,
    "writes_user_data": false,
    "requires_hitl": false,
    "is_blocked": false
}',
 true, 'intel.review.view'),

('intel.record.list',
 'Browse intelligence records with optional source_id, status, and entity_type filters (max 25 per call, newest first). Payloads are omitted — call intel.record.get for the full extraction.',
 'agent_intelligence',
 '{
    "function_name": "intel.record.list",
    "description": "List intelligence records with optional source/status/entity_type filters. Payloads omitted; call intel.record.get for the full extraction.",
    "parameters": {
        "type": "object",
        "properties": {
            "source_id":   { "type": "string", "description": "Optional UUID — only records from this source" },
            "status":      { "type": "string", "description": "Optional status filter (extracted, review_required, approved, rejected, published)" },
            "entity_type": { "type": "string", "description": "Optional entity type filter (e.g. scholarship, job, news)" },
            "limit":       { "type": "integer", "description": "Page size, max 25 (default 25)" },
            "offset":      { "type": "integer", "description": "Row offset for pagination (default 0)" }
        },
        "required": []
    },
    "handler": "IntelRecordList",
    "reads_user_data": false,
    "writes_user_data": false,
    "requires_hitl": false,
    "is_blocked": false
}',
 true, 'intel.review.view')
ON CONFLICT (name) DO NOTHING;

COMMIT;
