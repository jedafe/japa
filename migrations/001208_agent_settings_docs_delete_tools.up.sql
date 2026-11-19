-- 001208_agent_settings_docs_delete_tools
-- Agent tool catalog additions (Section 103):
--
-- 1. docs.delete — agent equivalent of DELETE /admin/docs/pages/{id}
--    (archives the page). Destructive: requires_hitl so the call lands in
--    the review queue. Mirrors the docs.* tool family's docs.admin gate.
--
-- 2. settings.get / settings.set — single-key system_settings read/write.
--    The handlers refuse uncatalogued keys, sensitive keys, deprecated
--    reachability toggles (managed via feature_flags), and security.* keys
--    (mirrors the blocked "settings.security.*" tool-namespace rule).
--    settings.get carries the REST read gate (system.settings.view);
--    settings.set carries the write gate (system.settings.edit) and
--    requires_hitl so writes land in the review queue.

BEGIN;

INSERT INTO ai_tools (name, description, category, config, is_enabled, required_permission) VALUES
('docs.delete',
 'Archive a documentation page (agent equivalent of DELETE /admin/docs/pages/{id}). Destructive — requires human review.',
 'agent_docs',
 '{"function_name":"docs.delete","description":"Archive a documentation page.","handler":"docs.delete","parameters":{"type":"object","properties":{"doc_id":{"type":"string","description":"Doc page UUID"}},"required":["doc_id"],"additionalProperties":false},"writes_user_data":false,"reads_user_data":false,"requires_hitl":true,"is_blocked":false}'::jsonb,
 true, 'docs.admin'),

('settings.get',
 'Read one system_setting by dot-notation key. Returns the parsed JSON value plus catalog metadata. Refuses uncatalogued, sensitive, and security.* keys.',
 'agent_settings',
 '{"function_name":"settings.get","description":"Read one system_setting by dot-notation key.","handler":"settings.get","parameters":{"type":"object","properties":{"key":{"type":"string","description":"Dot-notation setting key (e.g. docs.categories)"}},"required":["key"],"additionalProperties":false},"writes_user_data":false,"reads_user_data":false,"requires_hitl":false,"is_blocked":false}'::jsonb,
 true, 'system.settings.view'),

('settings.set',
 'Write one system_setting by dot-notation key. Value must satisfy the catalog type/bounds/options. Refuses uncatalogued, sensitive, deprecated (feature-flag-managed), and security.* keys. is_public resolves from the catalog, never the caller. Requires human review.',
 'agent_settings',
 '{"function_name":"settings.set","description":"Write one system_setting by dot-notation key.","handler":"settings.set","parameters":{"type":"object","properties":{"key":{"type":"string","description":"Dot-notation setting key (e.g. docs.categories)"},"value":{"description":"JSON value for the setting — must satisfy the catalog type"}},"required":["key","value"],"additionalProperties":false},"writes_user_data":false,"reads_user_data":false,"requires_hitl":true,"is_blocked":false}'::jsonb,
 true, 'system.settings.edit')
ON CONFLICT (name) DO UPDATE SET
    description = EXCLUDED.description,
    category = EXCLUDED.category,
    config = EXCLUDED.config,
    is_enabled = EXCLUDED.is_enabled,
    required_permission = EXCLUDED.required_permission;

COMMIT;
