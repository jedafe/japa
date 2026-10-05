-- Ref: japaconnect-engineering/sections/80-destination-wiki.md
--      (Amendment — Wiki-Only Space Types & Attractions)
--      docs/superpowers/specs/2026-10-05-wiki-attractions-design.md
--
-- space_types.wiki_only marks a type's spaces as wiki-surface-only (no feed,
-- join, or posting surface). Seeds the 'attraction' wiki-only type (child of
-- city/state/country) with an overview + plan-your-visit category template,
-- and adds an 'attractions' directory-mode category to the geo chain so
-- attraction spaces roll up as image-first cards on their parents.

BEGIN;

ALTER TABLE space_types ADD COLUMN wiki_only boolean NOT NULL DEFAULT false;

INSERT INTO space_types (id, name, slug, description, icon, field_group_id, is_system, category, wiki_only) VALUES
    ('00000000-0000-0000-0000-000000000071', 'Attraction', 'attraction',
     'Tourist attraction or natural wonder — wiki-only space', 'landmark',
     NULL, true, 'geo', true)
ON CONFLICT (id) DO NOTHING;

INSERT INTO wiki_type_configs (space_type_id, enabled, label, icon, seed_stubs, position) VALUES
    ('00000000-0000-0000-0000-000000000071', true, 'Attractions', 'landmark', true, 5)
ON CONFLICT (space_type_id) DO NOTHING;

-- Attraction nests under any geo level — city preferred; state/country direct
-- for rural or border attractions (national parks, waterfalls).
INSERT INTO wiki_type_parents (space_type_id, parent_space_type_id) VALUES
    ('00000000-0000-0000-0000-000000000071', '00000000-0000-0000-0000-000000000060'), -- attraction → country
    ('00000000-0000-0000-0000-000000000071', '00000000-0000-0000-0000-000000000061'), -- attraction → state
    ('00000000-0000-0000-0000-000000000071', '00000000-0000-0000-0000-000000000062')  -- attraction → city
ON CONFLICT DO NOTHING;

-- ---------------------------------------------------------------------------
-- Category seeds: two sets.
--  a) attraction's own template: overview (profile + gallery) and
--     plan-your-visit (visitor prose fields).
--  b) 'attractions' directory-mode categories on country/state/city —
--     rollup.mode='directory', levels=['attraction']; empty groups.
-- Group slug derives from category (underscored): wiki_{type}_{category}.
-- ---------------------------------------------------------------------------
CREATE TEMP TABLE _attr_cat_seed (
    type_id  uuid,
    slug     text,
    category text,
    label    text,
    position int,
    rollup   jsonb,
    icon     text
);

INSERT INTO _attr_cat_seed (type_id, slug, category, label, position, rollup, icon) VALUES
    -- attraction type template
    ('00000000-0000-0000-0000-000000000071', 'overview',        'overview',        'Overview',        1, NULL, 'info'),
    ('00000000-0000-0000-0000-000000000071', 'plan-your-visit', 'plan_your_visit', 'Plan Your Visit', 2, NULL, 'map'),
    -- directory categories on the geo chain (positions continue past the
    -- existing seeds: country ends at 8, state/city at 6 — migration 000739)
    ('00000000-0000-0000-0000-000000000060', 'attractions', 'attractions', 'Attractions', 9,
     '{"enabled":true,"mode":"directory","levels":["attraction"]}'::jsonb, 'landmark'),
    ('00000000-0000-0000-0000-000000000061', 'attractions', 'attractions', 'Attractions', 7,
     '{"enabled":true,"mode":"directory","levels":["attraction"]}'::jsonb, 'landmark'),
    ('00000000-0000-0000-0000-000000000062', 'attractions', 'attractions', 'Attractions', 7,
     '{"enabled":true,"mode":"directory","levels":["attraction"]}'::jsonb, 'landmark');

INSERT INTO custom_field_groups (name, slug, is_system)
SELECT c.label || ' (' || st.slug || ')',
       'wiki_' || st.slug || '_' || c.category,
       true
FROM _attr_cat_seed c
JOIN space_types st ON st.id = c.type_id
ON CONFLICT (slug) DO NOTHING;

INSERT INTO wiki_type_categories (id, space_type_id, slug, category, label, field_group_id, position, is_system, rollup, icon)
SELECT gen_random_uuid(), c.type_id, c.slug, c.category, c.label,
       g.id, c.position, true, c.rollup, c.icon
FROM _attr_cat_seed c
JOIN space_types st ON st.id = c.type_id
JOIN custom_field_groups g ON g.slug = 'wiki_' || st.slug || '_' || c.category
ON CONFLICT (space_type_id, slug) DO NOTHING;

-- Scope each category's group to its wiki type's entity class (000739 tail).
INSERT INTO custom_field_group_assignments (group_id, entity_class, entity_id, position)
SELECT DISTINCT wtc.field_group_id, 'space_type:' || st.slug, NULL::uuid, 0
FROM wiki_type_categories wtc
JOIN space_types st ON st.id = wtc.space_type_id
WHERE NOT EXISTS (
    SELECT 1 FROM custom_field_group_assignments a
    WHERE a.group_id = wtc.field_group_id
      AND a.entity_class = 'space_type:' || st.slug
      AND a.entity_id IS NULL
);

-- Attraction overview field schema (wiki_attraction_overview group).
-- Deterministic UUIDs (…0111–…0118, next free after baseline …0110) so
-- enrichAttractionFields + the wiki:seed_attractions dedup upsert can address
-- fields without a lookup — mirrors universityFieldUUIDs (processoradapter.go).
INSERT INTO custom_fields (id, group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position, config)
SELECT v.id, g.id, v.label, v.key, v.field_type, v.options, false, true, true, v.filterable, false, true, v.position, '{}'::jsonb
FROM custom_field_groups g
CROSS JOIN (VALUES
    ('00000000-0000-0000-0000-000000000111'::uuid, 'Kind',           'kind',          'select',      '[{"value":"museum","label":"Museum"},{"value":"landmark","label":"Landmark"},{"value":"monument","label":"Monument"},{"value":"historic-site","label":"Historic Site"},{"value":"religious-site","label":"Religious Site"},{"value":"market","label":"Market"},{"value":"waterfall","label":"Waterfall"},{"value":"mountain","label":"Mountain"},{"value":"rock-formation","label":"Rock Formation"},{"value":"lake","label":"Lake"},{"value":"cave","label":"Cave"},{"value":"beach","label":"Beach"},{"value":"forest-reserve","label":"Forest Reserve"},{"value":"canyon","label":"Canyon"},{"value":"hot-spring","label":"Hot Spring"},{"value":"national-park","label":"National Park"},{"value":"garden","label":"Garden"},{"value":"zoo","label":"Zoo"},{"value":"theme-park","label":"Theme Park"},{"value":"other","label":"Other"}]'::jsonb, true,  1),
    ('00000000-0000-0000-0000-000000000112'::uuid, 'Address',        'address',       'text',        NULL::jsonb, false, 2),
    ('00000000-0000-0000-0000-000000000113'::uuid, 'Coordinates',    'coordinates',   'text',        NULL::jsonb, false, 3),
    ('00000000-0000-0000-0000-000000000114'::uuid, 'Opening Hours',  'opening_hours', 'textarea',    NULL::jsonb, false, 4),
    ('00000000-0000-0000-0000-000000000115'::uuid, 'Entry Fee',      'entry_fee',     'price_range', NULL::jsonb, false, 5),
    ('00000000-0000-0000-0000-000000000116'::uuid, 'Website',        'website',       'url',         NULL::jsonb, false, 6),
    ('00000000-0000-0000-0000-000000000117'::uuid, 'Wikidata ID',    'wikidata_id',   'text',        NULL::jsonb, false, 7),
    ('00000000-0000-0000-0000-000000000118'::uuid, 'Gallery',        'gallery',       'image_multi', '{"max_files":12,"max_size_mb":8}'::jsonb, false, 8)
) AS v(id, label, key, field_type, options, filterable, position)
WHERE g.slug = 'wiki_attraction_overview'
  AND NOT EXISTS (
      SELECT 1 FROM custom_fields cf WHERE cf.group_id = g.id AND cf.key = v.key
  );

-- Plan Your Visit field schema (wiki_attraction_plan_your_visit group).
INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position, config)
SELECT g.id, v.label, v.key, 'textarea', NULL::jsonb, false, true, true, false, false, true, v.position, '{}'::jsonb
FROM custom_field_groups g
CROSS JOIN (VALUES
    ('Getting There', 'getting_there', 1),
    ('Best Time',     'best_time',     2),
    ('Tips',          'tips',          3)
) AS v(label, key, position)
WHERE g.slug = 'wiki_attraction_plan_your_visit'
  AND NOT EXISTS (
      SELECT 1 FROM custom_fields cf WHERE cf.group_id = g.id AND cf.key = v.key
  );

DROP TABLE _attr_cat_seed;

COMMIT;
