-- 000394_seed_ai_agent_comparison_tools.up.sql
-- Section 103 (AI Agent Executor) — Phase 7 Comparison Engine tool catalog.
--
-- Seeds the 5 comparison tools into the ai_tools table. Each row's
-- config JSONB contains the function schema (parameters), the handler
-- reference string (the Go handler registered in ToolHandlerRegistry),
-- and the behavioral flags (requires_hitl, reads_user_data,
-- writes_user_data, is_blocked) that drive the executor pipeline.
--
-- The required_permission column holds the RBAC node the executor's
-- RBAC gate checks (Phase 3, executor.go step 3). Read-only tools
-- (comparison.types.list, comparison.read) have no required_permission
-- and are available to every user who holds agent.use. Write tools
-- require the corresponding comparison.* permission node seeded by
-- migration 000099.
--
-- NOTE: ROADMAP-AI-AGENT.md named this migration 000372, but 000372 was
-- already used (ai_agent_privacy_sessions). 000393 was also already
-- used (seed_coupons_view_rbac_node). This migration uses 000394.
-- Flagged per AGENTS.md Rule 1.
--
-- Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Comparison Engine, Tool Catalog)
-- Ref: japaconnect-engineering/sections/30-generic-comparison-system.md
-- Ref: japaconnect-engineering/roadmaps/ROADMAP-AI-AGENT.md (Phase 7, task 7.06)

BEGIN;

-- =====================================================================
-- Comparison tools (Phase 7, task 7.06)
-- =====================================================================

INSERT INTO ai_tools (tenant_id, name, description, category, config, is_enabled, required_permission) VALUES
('00000000-0000-0000-0000-000000000001', 'comparison.types.list',
 'List comparison type definitions (e.g. universities, visa-types, banks). Use this to discover existing types before creating a new comparison.',
 'agent_comparison',
 '{
    "function_name": "comparison.types.list",
    "description": "List comparison type definitions for the tenant.",
    "parameters": {
        "type": "object",
        "properties": {
            "limit": { "type": "integer", "description": "Max results (default 50, max 100)" },
            "offset": { "type": "integer", "description": "Pagination offset (default 0)" }
        },
        "required": []
    },
    "handler": "ComparisonTypesList",
    "reads_user_data": false,
    "writes_user_data": false,
    "requires_hitl": false,
    "is_blocked": false
}',
 true, NULL),

('00000000-0000-0000-0000-000000000001', 'comparison.create',
 'Create a new comparison content row with items and dimensions. When create_type is provided, a new comparison type definition is created first (Tier 3). When entity_type references an existing type slug, the comparison is linked to it (Tier 2). The status is always draft — the agent never publishes directly.',
 'agent_comparison',
 '{
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
            "depth": { "type": "string", "description": "JCE comparison depth: 'quick' or 'detailed' (optional; default 'quick' when item_ref is present)" },
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
}',
 true, 'comparison.create'),

('00000000-0000-0000-0000-000000000001', 'comparison.fill_values',
 'Fill comparison values for items x dimensions. Each value records its source (custom_field, rag, external, or llm_knowledge), source_ref, confidence, and retrieved_at timestamp in the comparison_values metadata JSONB for provenance tracking.',
 'agent_comparison',
 '{
    "function_name": "comparison.fill_values",
    "description": "Fill comparison values with provenance metadata.",
    "parameters": {
        "type": "object",
        "properties": {
            "content_id": { "type": "string", "description": "Comparison content UUID" },
            "values": { "type": "array", "items": { "type": "object", "properties": {
                "dimension_id": { "type": "string", "description": "Dimension UUID" },
                "item_id": { "type": "string", "description": "Item UUID" },
                "value": { "type": "string", "description": "Text value" },
                "value_rating": { "type": "number", "description": "Numeric rating value" },
                "value_boolean": { "type": "boolean", "description": "Boolean value" },
                "source": { "type": "string", "description": "Provenance source: custom_field|rag|external|llm_knowledge" },
                "source_ref": { "type": "string", "description": "Source reference (URL, content UUID, or field value UUID)" },
                "confidence": { "type": "string", "description": "Confidence level: high|medium|low" }
            }, "required": ["dimension_id", "item_id", "source"] }, "description": "Values to fill" }
        },
        "required": ["content_id", "values"]
    },
    "handler": "ComparisonFillValues",
    "reads_user_data": false,
    "writes_user_data": true,
    "requires_hitl": false,
    "is_blocked": false
}',
 true, 'comparison.value.manage'),

('00000000-0000-0000-0000-000000000001', 'comparison.add_dimension',
 'Add one or more dimensions to an existing comparison content row (Tier 2 — semi-structured). Use after comparison.types.list identifies a type that is missing requested dimensions.',
 'agent_comparison',
 '{
    "function_name": "comparison.add_dimension",
    "description": "Add dimensions to an existing comparison.",
    "parameters": {
        "type": "object",
        "properties": {
            "content_id": { "type": "string", "description": "Comparison content UUID" },
            "dimensions": { "type": "array", "items": { "type": "object", "properties": {
                "label": { "type": "string", "description": "Dimension label" },
                "dimension_type": { "type": "string", "description": "Dimension type: text|rating|boolean|number|currency (default text)" }
            }, "required": ["label"] }, "description": "Dimensions to add" }
        },
        "required": ["content_id", "dimensions"]
    },
    "handler": "ComparisonAddDimension",
    "reads_user_data": false,
    "writes_user_data": true,
    "requires_hitl": true,
    "is_blocked": false
}',
 true, 'comparison.dimension.manage'),

('00000000-0000-0000-0000-000000000001', 'comparison.read',
 'Read a comparison with its full items x dimensions value matrix. Returns the parent content metadata, items, dimensions, and all cell values. Content marked ai_exempt is blocked by the privacy gate.',
 'agent_comparison',
 '{
    "function_name": "comparison.read",
    "description": "Read a comparison with its full value matrix.",
    "parameters": {
        "type": "object",
        "properties": {
            "content_id": { "type": "string", "description": "Comparison content UUID" }
        },
        "required": ["content_id"]
    },
    "handler": "ComparisonRead",
    "reads_user_data": true,
    "writes_user_data": false,
    "requires_hitl": false,
    "is_blocked": false
}',
 true, NULL)
ON CONFLICT (tenant_id, name) DO UPDATE
SET description = EXCLUDED.description,
    category = EXCLUDED.category,
    config = EXCLUDED.config,
    is_enabled = EXCLUDED.is_enabled,
    required_permission = EXCLUDED.required_permission,
    updated_at = now();

COMMIT;
