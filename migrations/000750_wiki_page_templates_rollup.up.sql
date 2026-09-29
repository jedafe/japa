-- 000750_wiki_page_templates_rollup.up.sql
-- §80 Hierarchical UX & Itemized Pricing amendment: a second template layer
-- (page templates under category templates) so sibling pages like "Prices in
-- {City}" bind a shared type-level group; categories may opt into descendant
-- rollups; custom_fields.section sub-groups fields inside a group.
--
-- Type edges become load-bearing: wiki_type_parents scopes rollup descendant
-- types, arbitrates multi-parent breadcrumb chains, and drives the /wiki
-- landing drill-down. Actual parentage still lives on spaces.parent_id /
-- space_parent_spaces.
--
-- Ref: japaconnect-engineering/sections/80-destination-wiki.md
--      docs/superpowers/specs/2026-09-28-wiki-hierarchical-ux-and-itemized-pricing-design.md

BEGIN;

-- Page template: a named sibling page under a category ("prices" under
-- cost_of_living). One row per (type, category, slug) — the group it binds
-- is shared by every space of the type, so item field keys stay comparable.
CREATE TABLE wiki_type_page_templates (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    category_id    uuid NOT NULL REFERENCES wiki_type_categories(id) ON DELETE CASCADE,
    slug           text NOT NULL,
    label          text NOT NULL,
    field_group_id uuid NOT NULL REFERENCES custom_field_groups(id) ON DELETE RESTRICT,
    position       int NOT NULL DEFAULT 0,
    is_system      boolean NOT NULL DEFAULT false,
    created_at     timestamptz NOT NULL DEFAULT now(),
    UNIQUE (category_id, slug)
);

-- Pages may bind a template (and through it a page-level group). The
-- resolved field_group_id is stored on the row so the binding survives
-- template deletion; NULLs fall back to the category's group.
ALTER TABLE wiki_pages
    ADD COLUMN page_template_id uuid REFERENCES wiki_type_page_templates(id) ON DELETE SET NULL,
    ADD COLUMN field_group_id   uuid REFERENCES custom_field_groups(id) ON DELETE RESTRICT,
    ADD COLUMN position         int NOT NULL DEFAULT 0;

-- Opt-in descendant aggregation per category. Shape:
--   {"enabled": true, "metrics": ["min","avg","max"], "levels": ["city"]}
-- levels omitted → descendant types derived from wiki_type_parents closure.
ALTER TABLE wiki_type_categories ADD COLUMN rollup jsonb;

-- Sub-grouping inside a field group: fields sharing a section token render
-- as one item table / one sub-tab (e.g. 'transportation' on wiki_city_prices).
ALTER TABLE custom_fields ADD COLUMN section text;

-- ---------------------------------------------------------------------------
-- Seeds.
-- 1. City "prices" page template under cost_of_living → dedicated shared
--    group wiki_city_prices + space_type:city assignment (empty container —
--    item fields are added by admins/ops, same convention as 000739).
-- 2. Rollup enabled on country/state cost_of_living (cities roll up).
-- ---------------------------------------------------------------------------

INSERT INTO custom_field_groups (name, slug, is_system)
SELECT 'Prices (city)', 'wiki_city_prices', true
WHERE NOT EXISTS (
    SELECT 1 FROM custom_field_groups WHERE slug = 'wiki_city_prices'
);

INSERT INTO custom_field_group_assignments (group_id, entity_class, entity_id, position)
SELECT g.id, 'space_type:' || st.slug, NULL::uuid, 0
FROM custom_field_groups g
JOIN space_types st ON st.slug = 'city'
WHERE g.slug = 'wiki_city_prices'
  AND NOT EXISTS (
      SELECT 1 FROM custom_field_group_assignments a
      WHERE a.group_id = g.id AND a.entity_class = 'space_type:' || st.slug AND a.entity_id IS NULL
  );

INSERT INTO wiki_type_page_templates (category_id, slug, label, field_group_id, position, is_system)
SELECT c.id, 'prices', 'Prices in {name}', g.id, 1, true
FROM wiki_type_categories c
JOIN space_types st ON st.id = c.space_type_id AND st.slug = 'city'
JOIN custom_field_groups g ON g.slug = 'wiki_city_prices'
WHERE c.category = 'cost_of_living'
ON CONFLICT (category_id, slug) DO NOTHING;

UPDATE wiki_type_categories
SET rollup = '{"enabled": true, "metrics": ["min","avg","max"]}'::jsonb
WHERE category = 'cost_of_living'
  AND space_type_id IN (
      SELECT st.id FROM space_types st WHERE st.slug IN ('country', 'state')
  );

-- JCE binding: a "City Prices" comparison type projects its dimensions from
-- the shared wiki_city_prices group, so any sectioned item field
-- (transportation, food, ...) is comparable city↔city. Section-scoped
-- comparisons are made by cloning this type with config.sections set.
INSERT INTO comparison_type_definitions (name, slug, description, field_group_id, item_label, is_system)
SELECT 'City Prices', 'city-prices',
       'Compare itemized cost-of-living prices between cities',
       g.id, 'City', true
FROM custom_field_groups g
WHERE g.slug = 'wiki_city_prices'
ON CONFLICT (slug) DO NOTHING;

-- internship/work_career are advertised by the source-proposal submittable
-- set (§80 gating) but had no category→method map entries — proposals
-- resolved to unrestricted probes and skipped materialization ("entity_types
-- empty after category-map fill"). Merge job-family aliases so both resolve
-- like job sources (job-family pages via JSON-LD/static + job extractor).
UPDATE system_settings
SET value = value || $cat${
  "internship": {
    "entity_types": ["job", "internship"],
    "methods": ["jsonld", "api", "tabular", "render", "static"],
    "pipeline": "extract",
    "topics": ["internship", "placement", "co-op", "graduate program", "industrial attachment", "trainee"]
  },
  "work_career": {
    "entity_types": ["job", "work_career"],
    "methods": ["jsonld", "api", "tabular", "render", "static"],
    "pipeline": "extract",
    "topics": ["career", "employment", "work abroad", "skilled worker", "job seeker", "labour market"]
  }
}$cat$::jsonb,
    updated_at = now()
WHERE key = 'intel.category_method_map';

COMMIT;
