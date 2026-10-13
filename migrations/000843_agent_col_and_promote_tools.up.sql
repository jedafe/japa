-- 000843_agent_col_and_promote_tools.up.sql
--
-- Two additions to the AI agent tool catalog (§103 / §30 JCE / §72 COL):
--
-- 1. col.city.get — canonical single-city cost-of-living read: the city
--    comparison_item (intel_cost_data), the dimension-value matrix, and the
--    §80 community custom_field_values overlay merged per-key. Closes the
--    read gap for COL agent flows (agent could fill values but not read
--    the canonical city record).
-- 2. comparison.promote — agent parity with POST
--    /api/v1/jce/comparisons/{id}/promote: quick → detailed draft.
--
-- required_permission is NULL on both: col.city.get reads the same public
-- dataset the COL calculator serves, and REST promote only requires auth —
-- ownership/comparison.edit_any is enforced inside the handler, matching
-- the REST route exactly.

BEGIN;

INSERT INTO ai_tools (name, description, category, config, is_enabled, required_permission) VALUES
('col.city.get',
 'Read a city''s canonical cost-of-living record: the city comparison_item with intel_cost_data, the full dimension-value matrix, and community wiki field values merged in (community values win per key, same as the COL calculator).',
 'agent_data',
 '{
    "function_name": "col.city.get",
    "description": "Read a city''s canonical cost-of-living record including intel cost data, community field overlay, and the dimension-value matrix.",
    "parameters": {
        "type": "object",
        "properties": {
            "city_id":      { "type": "string", "description": "UUID of the cost_of_living comparison_item (the city)" },
            "geo_space_id": { "type": "string", "description": "UUID of the city''s geo space — alternative to city_id" }
        },
        "required": []
    },
    "handler": "COLCityGet",
    "reads_user_data": false,
    "writes_user_data": false,
    "requires_hitl": false,
    "is_blocked": false
}',
 true, NULL),

('comparison.promote',
 'Promote a quick JCE comparison to a new detailed draft, cloning items and dimensions. Callers may promote their own comparisons; comparison.edit_any holders may promote anyone''s. The result is a draft — publish via content.publish.',
 'agent_comparison',
 '{
    "function_name": "comparison.promote",
    "description": "Promote a quick comparison to a detailed draft, cloning items and dimensions into a new comparison content row.",
    "parameters": {
        "type": "object",
        "properties": {
            "content_id": { "type": "string", "description": "UUID of the quick comparison content to promote" }
        },
        "required": ["content_id"]
    },
    "handler": "ComparisonPromote",
    "reads_user_data": false,
    "writes_user_data": true,
    "requires_hitl": false,
    "is_blocked": false
}',
 true, NULL)
ON CONFLICT (name) DO NOTHING;

COMMIT;
