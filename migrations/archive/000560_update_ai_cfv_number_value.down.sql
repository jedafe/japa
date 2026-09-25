-- 000560_update_ai_cfv_number_value.down.sql
-- Reverts the value_number schema additions for custom_field_values.set and
-- custom_field_values.bulk_set.

BEGIN;

UPDATE ai_tools
SET config = '{
    "function_name": "custom_field_values.set",
    "description": "Set a single custom field value. The target entity is privacy-checked before the write.",
    "parameters": {
        "type": "object",
        "properties": {
            "field_id": { "type": "string", "description": "Field definition UUID" },
            "entity_type": { "type": "string", "description": "Entity type" },
            "entity_id": { "type": "string", "description": "Entity UUID" },
            "value_text": { "type": "string", "description": "Text value" },
            "value_bool": { "type": "boolean", "description": "Boolean value" },
            "value_date": { "type": "string", "description": "Date value (RFC 3339)" },
            "value_json": { "type": "object", "description": "JSON value" }
        },
        "required": ["field_id", "entity_type", "entity_id"]
    },
    "handler": "CFVSet",
    "reads_user_data": true,
    "writes_user_data": true,
    "requires_hitl": false,
    "is_blocked": false
}'::jsonb
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND name = 'custom_field_values.set';

UPDATE ai_tools
SET config = '{
    "function_name": "custom_field_values.bulk_set",
    "description": "Set multiple custom field values for an entity.",
    "parameters": {
        "type": "object",
        "properties": {
            "entity_type": { "type": "string", "description": "Entity type" },
            "entity_id": { "type": "string", "description": "Entity UUID" },
            "values": {
                "type": "array",
                "description": "Field value entries",
                "items": {
                    "type": "object",
                    "properties": {
                        "field_id": { "type": "string", "description": "Field definition UUID" },
                        "value_text": { "type": "string" },
                        "value_bool": { "type": "boolean" },
                        "value_date": { "type": "string" },
                        "value_json": { "type": "object" }
                    },
                    "required": ["field_id"]
                }
            }
        },
        "required": ["entity_type", "entity_id", "values"]
    },
    "handler": "CFVBulkSet",
    "reads_user_data": true,
    "writes_user_data": true,
    "requires_hitl": true,
    "is_blocked": false
}'::jsonb
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND name = 'custom_field_values.bulk_set';

COMMIT;
