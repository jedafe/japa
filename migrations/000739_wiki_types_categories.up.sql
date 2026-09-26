-- 000739_wiki_types_categories.up.sql
-- §80 Wiki Types & Field-Group Pages amendment: wiki types become managed
-- rows over space_types (geo chain country→state→city is primary; other
-- types declare which geo types they nest under), and each wiki type gets a
-- category template where a category may bind a custom field group — so
-- every entity of a type shares the same page schema at
-- /wiki/{entity}/{slug}.
--
-- Seeds reproduce the formerly hardcoded category lists (geo set on
-- country/state/city; university set on university) with field_group_id
-- NULL — admins bind groups in the Admin Panel. slug = the page slug used in
-- /wiki/{entity}/{slug} URLs (hyphenated, matching the existing stub-page
-- convention); category = the wiki_pages.category token.
--
-- Ref: japaconnect-engineering/sections/80-destination-wiki.md
--      docs/superpowers/specs/2026-09-26-wiki-types-and-field-group-pages-design.md

BEGIN;

CREATE TABLE wiki_type_configs (
    space_type_id uuid PRIMARY KEY REFERENCES space_types(id),
    enabled       boolean NOT NULL DEFAULT true,
    label         text,
    icon          text,
    seed_stubs    boolean NOT NULL DEFAULT false,
    position      int NOT NULL DEFAULT 0,
    created_at    timestamptz NOT NULL DEFAULT now()
);

-- Which wiki types a type may nest under (allowed parentage for navigation
-- and admin validation; actual per-space parents live on
-- spaces.parent_id / space_parent_spaces).
CREATE TABLE wiki_type_parents (
    space_type_id        uuid NOT NULL REFERENCES wiki_type_configs(space_type_id) ON DELETE CASCADE,
    parent_space_type_id uuid NOT NULL REFERENCES wiki_type_configs(space_type_id) ON DELETE CASCADE,
    PRIMARY KEY (space_type_id, parent_space_type_id),
    CONSTRAINT wiki_type_parents_no_self CHECK (space_type_id <> parent_space_type_id)
);

-- Per-type category template. Every category owns a custom field group
-- (field_group_id NOT NULL): the group IS the page's schema — a prose-only
-- page is a category whose group has no fields. Each level's category binds
-- its own group scoped to entity_class 'space_type:{slug}' so JCE compares
-- like-for-like (city↔city, country↔country) — disjoint groups make
-- cross-level comparison unrepresentable. ON DELETE RESTRICT: a bound group
-- cannot be deleted out from under a category (DeleteFieldGroup → 409
-- in_use, same contract as ticket_types/comparison_type_definitions).
CREATE TABLE wiki_type_categories (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    space_type_id  uuid NOT NULL REFERENCES wiki_type_configs(space_type_id) ON DELETE CASCADE,
    slug           text NOT NULL,
    category       text NOT NULL,
    label          text NOT NULL,
    field_group_id uuid NOT NULL REFERENCES custom_field_groups(id) ON DELETE RESTRICT,
    position       int NOT NULL DEFAULT 0,
    is_system      boolean NOT NULL DEFAULT false,
    created_at     timestamptz NOT NULL DEFAULT now(),
    UNIQUE (space_type_id, slug),
    UNIQUE (space_type_id, category)
);

CREATE INDEX idx_wiki_type_categories_group
    ON wiki_type_categories (field_group_id);

-- ---------------------------------------------------------------------------
-- Seeds: wiki types for the geo chain + university.
-- Space type IDs (baseline seed): country=...060, state=...061, city=...062,
-- university=...064.
-- ---------------------------------------------------------------------------

INSERT INTO wiki_type_configs (space_type_id, enabled, label, icon, seed_stubs, position) VALUES
    ('00000000-0000-0000-0000-000000000060', true, NULL, NULL, true,  1),  -- country
    ('00000000-0000-0000-0000-000000000061', true, NULL, NULL, false, 2),  -- state
    ('00000000-0000-0000-0000-000000000062', true, NULL, NULL, false, 3),  -- city
    ('00000000-0000-0000-0000-000000000064', true, NULL, NULL, false, 4)   -- university
ON CONFLICT (space_type_id) DO NOTHING;

-- Geo chain: state nests under country; city nests under state. University
-- may nest under any geo level ("child of any or all" — matches
-- space_parent_spaces usage where universities hang under cities/states/
-- countries).
INSERT INTO wiki_type_parents (space_type_id, parent_space_type_id) VALUES
    ('00000000-0000-0000-0000-000000000061', '00000000-0000-0000-0000-000000000060'), -- state → country
    ('00000000-0000-0000-0000-000000000062', '00000000-0000-0000-0000-000000000061'), -- city  → state
    ('00000000-0000-0000-0000-000000000064', '00000000-0000-0000-0000-000000000060'), -- uni   → country
    ('00000000-0000-0000-0000-000000000064', '00000000-0000-0000-0000-000000000061'), -- uni   → state
    ('00000000-0000-0000-0000-000000000064', '00000000-0000-0000-0000-000000000062')  -- uni   → city
ON CONFLICT DO NOTHING;

-- Category template seeds + their dedicated per-level field groups.
-- Per-level fit: visa-routes and banking are country-only (visas and
-- banking systems are national); healthcare, cost-of-living, community,
-- housing, overview, and other exist at every geo level; university keeps
-- its own set.
--
-- Every category owns one empty custom_field_groups row named
-- wiki_{type_slug}_{category} (e.g. wiki_city_cost_of_living) plus a
-- custom_field_group_assignments row scoped to entity_class
-- 'space_type:{type_slug}'. Schema is type-level and cfv values are
-- entity-level, so JCE compares like-for-like (city↔city across countries,
-- never city↔country) — cross-level comparison is unrepresentable because
-- the groups are disjoint. A "prose-only" page is a category whose group
-- has no fields.
CREATE TEMP TABLE _wiki_cat_seed (
    type_id  uuid,
    slug     text,
    category text,
    label    text,
    position int
);

INSERT INTO _wiki_cat_seed (type_id, slug, category, label, position) VALUES
    -- country
    ('00000000-0000-0000-0000-000000000060', 'overview',       'overview',       'Overview',       1),
    ('00000000-0000-0000-0000-000000000060', 'visa-routes',    'visa',           'Visa Routes',    2),
    ('00000000-0000-0000-0000-000000000060', 'cost-of-living', 'cost_of_living', 'Cost of Living', 3),
    ('00000000-0000-0000-0000-000000000060', 'healthcare',     'healthcare',     'Healthcare',     4),
    ('00000000-0000-0000-0000-000000000060', 'banking',        'banking',        'Banking',        5),
    ('00000000-0000-0000-0000-000000000060', 'community',      'community',      'Community',      6),
    ('00000000-0000-0000-0000-000000000060', 'housing',        'housing',        'Housing',        7),
    ('00000000-0000-0000-0000-000000000060', 'other',          'other',          'Other',          8),
    -- state (no visa-routes or banking — national topics)
    ('00000000-0000-0000-0000-000000000061', 'overview',       'overview',       'Overview',       1),
    ('00000000-0000-0000-0000-000000000061', 'cost-of-living', 'cost_of_living', 'Cost of Living', 2),
    ('00000000-0000-0000-0000-000000000061', 'healthcare',     'healthcare',     'Healthcare',     3),
    ('00000000-0000-0000-0000-000000000061', 'community',      'community',      'Community',      4),
    ('00000000-0000-0000-0000-000000000061', 'housing',        'housing',        'Housing',        5),
    ('00000000-0000-0000-0000-000000000061', 'other',          'other',          'Other',          6),
    -- city (no visa-routes or banking — national topics)
    ('00000000-0000-0000-0000-000000000062', 'overview',       'overview',       'Overview',       1),
    ('00000000-0000-0000-0000-000000000062', 'cost-of-living', 'cost_of_living', 'Cost of Living', 2),
    ('00000000-0000-0000-0000-000000000062', 'healthcare',     'healthcare',     'Healthcare',     3),
    ('00000000-0000-0000-0000-000000000062', 'community',      'community',      'Community',      4),
    ('00000000-0000-0000-0000-000000000062', 'housing',        'housing',        'Housing',        5),
    ('00000000-0000-0000-0000-000000000062', 'other',          'other',          'Other',          6),
    -- university
    ('00000000-0000-0000-0000-000000000064', 'overview',       'overview',       'Overview',       1),
    ('00000000-0000-0000-0000-000000000064', 'admissions',     'admissions',     'Admissions',     2),
    ('00000000-0000-0000-0000-000000000064', 'programs',       'programs',       'Programs',       3),
    ('00000000-0000-0000-0000-000000000064', 'tuition-fees',   'tuition_fees',   'Tuition & Fees', 4),
    ('00000000-0000-0000-0000-000000000064', 'scholarships',   'scholarships',   'Scholarships',   5),
    ('00000000-0000-0000-0000-000000000064', 'accommodation',  'accommodation',  'Accommodation',  6),
    ('00000000-0000-0000-0000-000000000064', 'campus-life',    'campus_life',    'Campus Life',    7),
    ('00000000-0000-0000-0000-000000000064', 'careers',        'careers',        'Careers',        8),
    ('00000000-0000-0000-0000-000000000064', 'other',          'other',          'Other',          9);

-- Dedicated per-level groups: wiki_{type_slug}_{category}.
INSERT INTO custom_field_groups (name, slug, is_system)
SELECT c.label || ' (' || st.slug || ')',
       'wiki_' || st.slug || '_' || c.category,
       true
FROM _wiki_cat_seed c
JOIN space_types st ON st.id = c.type_id
ON CONFLICT (slug) DO NOTHING;

INSERT INTO wiki_type_categories (id, space_type_id, slug, category, label, field_group_id, position, is_system)
SELECT gen_random_uuid(), c.type_id, c.slug, c.category, c.label,
       g.id, c.position, true
FROM _wiki_cat_seed c
JOIN space_types st ON st.id = c.type_id
JOIN custom_field_groups g ON g.slug = 'wiki_' || st.slug || '_' || c.category
ON CONFLICT (space_type_id, slug) DO NOTHING;

-- Scope each category's group to exactly its wiki type's entity class so
-- field resolution, permissions, and ops validation treat it as the type's
-- class-level group.
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

DROP TABLE _wiki_cat_seed;

COMMIT;
