-- 000559_seed_ai_content_type_tools.down.sql
-- Reverses 000559: removes content_types.list and reverts
-- custom_field_groups.list to the no-parameter schema.

BEGIN;

DELETE FROM ai_tools
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND name = 'content_types.list';

UPDATE ai_tools
SET config = '{
    "function_name": "custom_field_groups.list",
    "description": "List custom field groups.",
    "parameters": {
        "type": "object",
        "properties": {}
    },
    "handler": "CustomFieldGroupsList",
    "reads_user_data": false,
    "writes_user_data": false,
    "requires_hitl": false,
    "is_blocked": false
}'::jsonb,
    description = 'List custom field groups for the tenant.'
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND name = 'custom_field_groups.list';

COMMIT;
