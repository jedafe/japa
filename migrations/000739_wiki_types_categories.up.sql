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

-- Per-type category template. field_group_id NULL = prose-only page.
CREATE TABLE wiki_type_categories (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    space_type_id  uuid NOT NULL REFERENCES wiki_type_configs(space_type_id) ON DELETE CASCADE,
    slug           text NOT NULL,
    category       text NOT NULL,
    label          text NOT NULL,
    field_group_id uuid REFERENCES custom_field_groups(id) ON DELETE SET NULL,
    position       int NOT NULL DEFAULT 0,
    is_system      boolean NOT NULL DEFAULT false,
    created_at     timestamptz NOT NULL DEFAULT now(),
    UNIQUE (space_type_id, slug),
    UNIQUE (space_type_id, category)
);

CREATE INDEX idx_wiki_type_categories_group
    ON wiki_type_categories (field_group_id) WHERE field_group_id IS NOT NULL;

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

-- Geo category set (was the hardcoded wikiCategories slice + stub seeder
-- spec). Per-level fit: visa-routes and banking are country-only (visas and
-- banking systems are national); healthcare, cost-of-living, community,
-- housing, overview, and other exist at every geo level.
INSERT INTO wiki_type_categories (id, space_type_id, slug, category, label, field_group_id, position, is_system) VALUES
    -- country
    ('00000000-0000-0000-0000-00000000d001', '00000000-0000-0000-0000-000000000060', 'overview',       'overview',       'Overview',        NULL, 1, true),
    ('00000000-0000-0000-0000-00000000d002', '00000000-0000-0000-0000-000000000060', 'visa-routes',    'visa',           'Visa Routes',     NULL, 2, true),
    ('00000000-0000-0000-0000-00000000d003', '00000000-0000-0000-0000-000000000060', 'cost-of-living', 'cost_of_living', 'Cost of Living',  NULL, 3, true),
    ('00000000-0000-0000-0000-00000000d004', '00000000-0000-0000-0000-000000000060', 'healthcare',     'healthcare',     'Healthcare',      NULL, 4, true),
    ('00000000-0000-0000-0000-00000000d005', '00000000-0000-0000-0000-000000000060', 'banking',        'banking',        'Banking',         NULL, 5, true),
    ('00000000-0000-0000-0000-00000000d006', '00000000-0000-0000-0000-000000000060', 'community',      'community',      'Community',       NULL, 6, true),
    ('00000000-0000-0000-0000-00000000d007', '00000000-0000-0000-0000-000000000060', 'housing',        'housing',        'Housing',         NULL, 7, true),
    ('00000000-0000-0000-0000-00000000d008', '00000000-0000-0000-0000-000000000060', 'other',          'other',          'Other',           NULL, 8, true),
    -- state (no visa-routes or banking — national topics)
    ('00000000-0000-0000-0000-00000000d011', '00000000-0000-0000-0000-000000000061', 'overview',       'overview',       'Overview',        NULL, 1, true),
    ('00000000-0000-0000-0000-00000000d013', '00000000-0000-0000-0000-000000000061', 'cost-of-living', 'cost_of_living', 'Cost of Living',  NULL, 2, true),
    ('00000000-0000-0000-0000-00000000d014', '00000000-0000-0000-0000-000000000061', 'healthcare',     'healthcare',     'Healthcare',      NULL, 3, true),
    ('00000000-0000-0000-0000-00000000d016', '00000000-0000-0000-0000-000000000061', 'community',      'community',      'Community',       NULL, 4, true),
    ('00000000-0000-0000-0000-00000000d017', '00000000-0000-0000-0000-000000000061', 'housing',        'housing',        'Housing',         NULL, 5, true),
    ('00000000-0000-0000-0000-00000000d018', '00000000-0000-0000-0000-000000000061', 'other',          'other',          'Other',           NULL, 6, true),
    -- city (no visa-routes or banking — national topics)
    ('00000000-0000-0000-0000-00000000d021', '00000000-0000-0000-0000-000000000062', 'overview',       'overview',       'Overview',        NULL, 1, true),
    ('00000000-0000-0000-0000-00000000d023', '00000000-0000-0000-0000-000000000062', 'cost-of-living', 'cost_of_living', 'Cost of Living',  NULL, 2, true),
    ('00000000-0000-0000-0000-00000000d024', '00000000-0000-0000-0000-000000000062', 'healthcare',     'healthcare',     'Healthcare',      NULL, 3, true),
    ('00000000-0000-0000-0000-00000000d026', '00000000-0000-0000-0000-000000000062', 'community',      'community',      'Community',       NULL, 4, true),
    ('00000000-0000-0000-0000-00000000d027', '00000000-0000-0000-0000-000000000062', 'housing',        'housing',        'Housing',         NULL, 5, true),
    ('00000000-0000-0000-0000-00000000d028', '00000000-0000-0000-0000-000000000062', 'other',          'other',          'Other',           NULL, 6, true),
    -- university (was the hardcoded wikiUniversityCategories slice)
    ('00000000-0000-0000-0000-00000000d031', '00000000-0000-0000-0000-000000000064', 'overview',       'overview',       'Overview',        NULL, 1, true),
    ('00000000-0000-0000-0000-00000000d032', '00000000-0000-0000-0000-000000000064', 'admissions',     'admissions',     'Admissions',      NULL, 2, true),
    ('00000000-0000-0000-0000-00000000d033', '00000000-0000-0000-0000-000000000064', 'programs',       'programs',       'Programs',        NULL, 3, true),
    ('00000000-0000-0000-0000-00000000d034', '00000000-0000-0000-0000-000000000064', 'tuition-fees',   'tuition_fees',   'Tuition & Fees',  NULL, 4, true),
    ('00000000-0000-0000-0000-00000000d035', '00000000-0000-0000-0000-000000000064', 'scholarships',   'scholarships',   'Scholarships',    NULL, 5, true),
    ('00000000-0000-0000-0000-00000000d036', '00000000-0000-0000-0000-000000000064', 'accommodation',  'accommodation',  'Accommodation',   NULL, 6, true),
    ('00000000-0000-0000-0000-00000000d037', '00000000-0000-0000-0000-000000000064', 'campus-life',    'campus_life',    'Campus Life',     NULL, 7, true),
    ('00000000-0000-0000-0000-00000000d038', '00000000-0000-0000-0000-000000000064', 'careers',        'careers',        'Careers',         NULL, 8, true),
    ('00000000-0000-0000-0000-00000000d039', '00000000-0000-0000-0000-000000000064', 'other',          'other',          'Other',           NULL, 9, true)
ON CONFLICT (space_type_id, slug) DO NOTHING;

COMMIT;
