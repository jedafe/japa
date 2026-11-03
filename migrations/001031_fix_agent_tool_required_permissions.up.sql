-- 001031_fix_agent_tool_required_permissions.up.sql
-- Three agent tools carry required_permission='content.update', a node that
-- was never seeded into permissions — so HasPermission() resolves false for
-- every user except '*' holders, and the node can never be granted in the
-- RBAC admin UI.
--
--   content.update            -> content.edit_any
--     The tool updates an arbitrary contents row by ID with no author check
--     (agent_adapters.go bypasses the HTTP handler's author||edit_any gate),
--     so it needs the any-scope node, not the author-scoped edit_own.
--   custom_field_values.set   -> custom_fields.values.set
--   custom_field_values.bulk_set -> custom_fields.values.set
--     The seeded node for "Set custom field values" — the previous value was
--     a copy/paste of the content tool's (unseeded) key.

BEGIN;

UPDATE ai_tools
SET required_permission = 'content.edit_any', updated_at = now()
WHERE name = 'content.update' AND required_permission = 'content.update';

UPDATE ai_tools
SET required_permission = 'custom_fields.values.set', updated_at = now()
WHERE name IN ('custom_field_values.set', 'custom_field_values.bulk_set')
  AND required_permission = 'content.update';

COMMIT;
