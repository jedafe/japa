-- 000541_update_ai_comparison_create_schema.down.sql
-- Reverts the comparison.create schema to the pre-JCE shape.

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
            "items": { "type": "array", "items": { "type": "object", "properties": { "name": { "type": "string" }, "slug": { "type": "string" } }, "required": ["name"] }, "description": "Items to compare" },
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
