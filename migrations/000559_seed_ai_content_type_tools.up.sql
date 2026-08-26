-- 000559_seed_ai_content_type_tools.up.sql
-- Section 103 (AI Agent Executor) — General content type discovery.
--
-- 1. Adds the content_types.list tool so the agent can discover every valid
--    content type and the entity_class to use with custom_field_groups.list.
-- 2. Updates custom_field_groups.list to accept an optional entity_class
--    filter (matching the HTTP endpoint's ?entity_class query parameter).
--
-- Ref: japaconnect-engineering/sections/103-ai-agent-executor.md
-- Ref: japaconnect-engineering/sections/14-content-type-system.md

BEGIN;

INSERT INTO ai_tools (tenant_id, name, description, category, config, is_enabled, required_permission) VALUES
('00000000-0000-0000-0000-000000000001', 'content_types.list',
 'List valid content types for the tenant. Returns content_type, label, has_category, category_list_tool, field_group_entity_class, field_group_id, and field_group_slug. Use this before creating any content.',
 'agent_content',
 '{
    "function_name": "content_types.list",
    "description": "List valid content types and their custom field group hints.",
    "parameters": {
        "type": "object",
        "properties": {}
    },
    "handler": "ContentTypesList",
    "reads_user_data": false,
    "writes_user_data": false,
    "requires_hitl": false,
    "is_blocked": false
}'::jsonb,
 true, NULL)
ON CONFLICT (tenant_id, name) DO UPDATE SET
    description = EXCLUDED.description,
    category = EXCLUDED.category,
    config = EXCLUDED.config,
    is_enabled = EXCLUDED.is_enabled,
    required_permission = EXCLUDED.required_permission;

UPDATE ai_tools
SET config = '{
    "function_name": "custom_field_groups.list",
    "description": "List custom field groups for the tenant, optionally filtered by entity_class.",
    "parameters": {
        "type": "object",
        "properties": {
            "entity_class": { "type": "string", "description": "Optional entity_class filter (e.g. content_type:blog_post, content_type:guide:relocation, content). Returns only groups assigned to this entity class." }
        },
        "required": []
    },
    "handler": "CustomFieldGroupsList",
    "reads_user_data": false,
    "writes_user_data": false,
    "requires_hitl": false,
    "is_blocked": false
}'::jsonb,
    description = 'List custom field groups for the tenant. Use the entity_class parameter to narrow results (e.g. content_type:blog_post).'
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND name = 'custom_field_groups.list';

COMMIT;
