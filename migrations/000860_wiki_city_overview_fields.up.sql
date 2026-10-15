-- 000860_wiki_city_overview_fields.up.sql
-- §80 Super-Admin Direct Publish amendment, follow-up: the space index
-- is the overview tab, so its fields panel / SuggestEdit / Quick Facts
-- all scope to the bound overview group — but wiki_city_overview had no
-- fields, leaving every city index with an empty "data" section and no
-- Quick Facts card. Seed the canonical city overview fields mirroring
-- the wiki_country_overview shape (community-editable, is_system=false),
-- grant the standard field permissions (all groups view, non-guest
-- edit), and seed Toronto's values so the surface renders immediately.
--
-- Ref: .jeos/sections/80-destination-wiki.md

BEGIN;

-- 1. Canonical city overview fields.
INSERT INTO custom_fields (group_id, key, label, field_type, options, position, is_system, show_in_frontend)
SELECT g.id, m.key, m.label, m.field_type, NULL::jsonb, m.position, false, true
FROM custom_field_groups g
JOIN (VALUES
	('population',        'Population',          'number',   1),
	('province_state',    'Province / State',    'text',     2),
	('timezone',          'Timezone',            'text',     3),
	('official_language', 'Official Language',   'text',     4),
	('popular_places',    'Popular Places',      'repeater', 5),
	('main_airport',      'Main Airport',        'text',     6)
) AS m(key, label, field_type, position) ON true
WHERE g.slug = 'wiki_city_overview'
ON CONFLICT (group_id, key) DO NOTHING;

-- 2. Field permissions matching the wiki_country_overview convention:
--    every group can view, every non-guest group can edit.
INSERT INTO custom_field_permissions (field_id, user_group_id, can_view, can_edit)
SELECT f.id, ug.id, true, ug.slug <> 'guest'
FROM custom_fields f
JOIN custom_field_groups g ON g.id = f.group_id AND g.slug = 'wiki_city_overview'
CROSS JOIN user_groups ug
WHERE f.key IN ('population', 'province_state', 'timezone', 'official_language', 'popular_places', 'main_airport')
ON CONFLICT (field_id, user_group_id) DO NOTHING;

-- 3. Toronto seed values (community-editable — the wiki review path
--    governs later corrections).
INSERT INTO custom_field_values (field_id, entity_type, entity_id, value_text, value_number, value_json, created_at, updated_at)
SELECT f.id, 'space', s.id,
	CASE m.key
		WHEN 'province_state'    THEN 'Ontario'
		WHEN 'timezone'          THEN 'Eastern Time (ET)'
		WHEN 'official_language' THEN 'English'
		WHEN 'main_airport'      THEN 'Toronto Pearson (YYZ)'
	END,
	CASE m.key WHEN 'population' THEN 2794356 END,
	CASE m.key WHEN 'popular_places' THEN '["CN Tower","Royal Ontario Museum","Ripley''s Aquarium","Distillery District"]'::jsonb END,
	now(), now()
FROM custom_fields f
JOIN custom_field_groups g ON g.id = f.group_id AND g.slug = 'wiki_city_overview'
JOIN (VALUES
	('population'), ('province_state'), ('timezone'), ('official_language'), ('popular_places'), ('main_airport')
) AS m(key) ON m.key = f.key
CROSS JOIN spaces s
WHERE s.slug = 'toronto'
ON CONFLICT DO NOTHING;

COMMIT;
