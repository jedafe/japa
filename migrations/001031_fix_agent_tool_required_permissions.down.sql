-- 001031_fix_agent_tool_required_permissions.down.sql
-- Restores the previous (unseeded) required_permission values.

BEGIN;

UPDATE ai_tools
SET required_permission = 'content.update', updated_at = now()
WHERE name IN ('content.update', 'custom_field_values.set', 'custom_field_values.bulk_set')
  AND required_permission IN ('content.edit_any', 'custom_fields.values.set');

COMMIT;
