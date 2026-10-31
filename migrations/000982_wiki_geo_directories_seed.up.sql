-- 000982_wiki_geo_directories_seed.up.sql
-- Ref: japaconnect-engineering/sections/80-destination-wiki.md (Directories)
-- Seed rollup-mode:"directory" categories so geo drill-down works end to end:
-- the landing's drill-down was replaced by directory pages on each parent,
-- but only 'attractions' (+ education 'programs') directories were seeded —
-- states/cities/universities were unreachable from parent wikis. Mirrors the
-- 000739 category-seed conventions: dedicated wiki_{type}_{category} field
-- group (empty — directories list children, not fields) + a class-wide
-- custom_field_group_assignments row on entity_class 'space_type:{slug}'.
--
-- Space type IDs (baseline seed): country=...060, state=...061, city=...062,
-- education=...066. Directory levels are descendant space-type slugs
-- resolved by computeDirectory (transitive: a country's cities list resolves
-- through its states; universities parent to country/state/city).

CREATE TEMP TABLE _wiki_dir_seed (
    type_id  uuid,
    slug     text,
    label    text,
    levels   text[],
    position int
);

INSERT INTO _wiki_dir_seed (type_id, slug, label, levels, position) VALUES
    -- country → states, cities (transitive), universities
    ('00000000-0000-0000-0000-000000000060', 'states',       '{name} States',       ARRAY['state'],     14),
    ('00000000-0000-0000-0000-000000000060', 'cities',       '{name} Cities',       ARRAY['city'],      15),
    ('00000000-0000-0000-0000-000000000060', 'universities', '{name} Universities', ARRAY['education'], 16),
    -- state → cities, universities
    ('00000000-0000-0000-0000-000000000061', 'cities',       '{name} Cities',       ARRAY['city'],      14),
    ('00000000-0000-0000-0000-000000000061', 'universities', '{name} Universities', ARRAY['education'], 15),
    -- city → universities
    ('00000000-0000-0000-0000-000000000062', 'universities', '{name} Universities', ARRAY['education'], 15);

-- Dedicated per-category field groups: wiki_{type_slug}_{category}.
INSERT INTO custom_field_groups (name, slug, is_system)
SELECT c.label || ' (' || st.slug || ')',
       'wiki_' || st.slug || '_' || c.slug,
       true
FROM _wiki_dir_seed c
JOIN space_types st ON st.id = c.type_id
ON CONFLICT (slug) DO NOTHING;

INSERT INTO wiki_type_categories (id, space_type_id, slug, category, label, field_group_id, position, is_system, rollup)
SELECT gen_random_uuid(), c.type_id, c.slug, c.slug, c.label,
       g.id, c.position, true,
       jsonb_build_object('mode', 'directory', 'levels', to_jsonb(c.levels), 'enabled', true)
FROM _wiki_dir_seed c
JOIN space_types st ON st.id = c.type_id
JOIN custom_field_groups g ON g.slug = 'wiki_' || st.slug || '_' || c.slug
ON CONFLICT (space_type_id, slug) DO NOTHING;

-- Scope each group to its wiki type's entity class (same convention as the
-- 000739 seeds) so field resolution treats it as the type's class group.
INSERT INTO custom_field_group_assignments (group_id, entity_class, entity_id, position)
SELECT DISTINCT wtc.field_group_id, 'space_type:' || st.slug, NULL::uuid, 0
FROM wiki_type_categories wtc
JOIN _wiki_dir_seed c ON c.type_id = wtc.space_type_id AND c.slug = wtc.slug
JOIN space_types st ON st.id = wtc.space_type_id
WHERE NOT EXISTS (
    SELECT 1 FROM custom_field_group_assignments a
    WHERE a.group_id = wtc.field_group_id
      AND a.entity_class = 'space_type:' || st.slug
      AND a.entity_id IS NULL
);

DROP TABLE _wiki_dir_seed;
