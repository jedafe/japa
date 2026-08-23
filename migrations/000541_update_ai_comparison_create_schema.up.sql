-- 000541_update_ai_comparison_create_schema.up.sql
-- Section 103 (AI Agent Executor) — update comparison.create schema to expose
-- JCE depth and polymorphic item_ref so the agent can produce quick/detailed
-- comparisons with resolver-backed item references.
--
-- Ref: japaconnect-engineering/sections/103-ai-agent-executor.md
-- Ref: japaconnect-engineering/sections/30-generic-comparison-system.md

BEGIN;

UPDATE ai_tools
SET config = '{
    "function_name": "comparison.create",
    "description": "Create a new comparison with items and dimensions.",
    "parameters": {
        "type": "object",
        "properties": {
            "title": { "type": "string", "description": "Comparison title" },
            "body": { "type": "string", "description": "Comparison body (markdown, optional)" },
            "entity_type": { "type": "string", "description": "Slug of an existing comparison type to link (optional)" },
            "visibility": { "type": "string", "description": "Visibility (default members)" },
            "space_id": { "type": "string", "description": "Optional space UUID" },
            "items": { "type": "array", "items": { "type": "object", "properties": { "name": { "type": "string" }, "slug": { "type": "string" }, "item_ref": { "type": "object", "description": "Polymorphic JCE item reference, e.g. {\"type\":\"university\", \"id\":\"...\"}" } }, "required": ["name"] }, "description": "Items to compare" },
            "depth": { "type": "string", "description": "JCE comparison depth: ''quick'' or ''detailed'' (optional; default ''quick'' when item_ref is present)" },
            "dimensions": { "type": "array", "items": { "type": "string" }, "description": "Dimension labels" },
            "create_type": { "type": "object", "properties": { "name": { "type": "string" }, "slug": { "type": "string" }, "description": { "type": "string" }, "item_label": { "type": "string" } }, "required": ["name", "slug"], "description": "Create a new comparison type definition (Tier 3)" }
        },
        "required": ["title", "items"]
    },
    "handler": "ComparisonCreate",
    "reads_user_data": false,
    "writes_user_data": true,
    "requires_hitl": true,
    "is_blocked": false
}'::jsonb
WHERE name = 'comparison.create';

COMMIT;
