-- 000971_wiki_program_type_related_mode.up.sql
-- University ↔ course/program cross-links (spec
-- 2026-10-30-wiki-university-course-program-links-design.md):
--   * 'program' entity kind + wiki_only type over
--     university_course_catalog (canonical "BSc Computer Engineering";
--     'course' stays offering-bound).
--   * program categories incl. 'universities' (rollup mode:"related").
--   * 'universities' category on the course (offering) type.
--   * university 'programs' category → directory-mode rollup over the
--     entity-bound 'course' child type (grid of offerings).
BEGIN;

INSERT INTO wiki_entity_kinds (entity_kind, source_table, id_column, slug_column, label) VALUES
    ('program', 'university_course_catalog', 'id', 'slug', 'Course Program')
ON CONFLICT (entity_kind) DO NOTHING;

INSERT INTO space_types (id, name, slug, description, icon, field_group_id, is_system, category, wiki_only) VALUES
    ('00000000-0000-0000-0000-00000000007e', 'Program', 'program',
     'Canonical course/program (entity-bound wiki type)', 'book-open',
     NULL, true, NULL, true)
ON CONFLICT (id) DO NOTHING;

INSERT INTO wiki_type_configs (space_type_id, enabled, label, icon, seed_stubs, position, entity_kind, entity_discriminator) VALUES
    ('00000000-0000-0000-0000-00000000007e', true, NULL, NULL, false, 18, 'program', NULL)
ON CONFLICT (space_type_id) DO NOTHING;
-- program gets NO wiki_type_parents edge: no single host space, and the
-- course→university edge already covers the child-listing axis.

CREATE TEMP TABLE _xlink_cat_seed (
    type_slug  text,
    slug       text,
    category   text,
    label      text,
    position   int,
    rollup     jsonb
) ON COMMIT DROP;

INSERT INTO _xlink_cat_seed (type_slug, slug, category, label, position, rollup) VALUES
    -- program (canonical catalog type)
    ('program', 'overview',        'overview',        'Overview',        1, NULL),
    ('program', 'universities',    'universities',    'Universities',    2, '{"enabled":true,"mode":"related"}'::jsonb),
    ('program', 'career-outcomes', 'career_outcomes', 'Career Outcomes', 3, NULL),
    -- course (offering type — positions continue past career-outcomes 4)
    ('course',  'universities',    'universities',    'Universities',    5, '{"enabled":true,"mode":"related"}'::jsonb);

INSERT INTO custom_field_groups (name, slug, is_system)
SELECT c.label || ' (' || c.type_slug || ')',
       'wiki_' || c.type_slug || '_' || c.category,
       true
FROM _xlink_cat_seed c
ON CONFLICT (slug) DO NOTHING;

INSERT INTO wiki_type_categories (id, space_type_id, slug, category, label, field_group_id, position, is_system, rollup)
SELECT gen_random_uuid(), st.id, c.slug, c.category, c.label, g.id, c.position, true, c.rollup
FROM _xlink_cat_seed c
JOIN space_types st ON st.slug = c.type_slug
JOIN custom_field_groups g ON g.slug = 'wiki_' || c.type_slug || '_' || c.category
ON CONFLICT (space_type_id, slug) DO NOTHING;

-- Entity-class assignments so cfv writes scope to the entity rows.
INSERT INTO custom_field_group_assignments (group_id, entity_class, entity_id, position)
SELECT DISTINCT wtc.field_group_id, cfg.entity_kind, NULL::uuid, 0
FROM wiki_type_categories wtc
JOIN space_types st ON st.id = wtc.space_type_id
JOIN wiki_type_configs cfg ON cfg.space_type_id = wtc.space_type_id
JOIN _xlink_cat_seed s ON s.type_slug = st.slug AND s.slug = wtc.slug
WHERE cfg.entity_kind IS NOT NULL
  AND NOT EXISTS (
      SELECT 1 FROM custom_field_group_assignments a
      WHERE a.group_id = wtc.field_group_id AND a.entity_class = cfg.entity_kind AND a.entity_id IS NULL
  );

-- University 'programs' category: directory-mode rollup listing the
-- entity-bound 'course' children (grid of this uni's offerings).
UPDATE wiki_type_categories c
SET rollup = '{"enabled":true,"mode":"directory","levels":["course"]}'::jsonb
FROM space_types st
WHERE c.space_type_id = st.id AND st.slug = 'university' AND c.slug = 'programs';

COMMIT;
