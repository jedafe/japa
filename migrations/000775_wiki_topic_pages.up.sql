-- 000775_wiki_topic_pages.up.sql
-- §80 Wiki Topic Pages amendment: page templates gain a parent_template_id
-- (one nesting level — the parent must be same-category and itself top-level,
-- enforced at the service layer), a section_label used when the child renders
-- as a composite preview section on the parent page, and preview_limit, the
-- number of child-group rows shown in that preview (0 = all).
--
-- Ref: japaconnect-engineering/sections/80-destination-wiki.md
--      docs/superpowers/specs/2026-10-02-wiki-topic-pages-design.md

BEGIN;

ALTER TABLE wiki_type_page_templates
    ADD COLUMN parent_template_id uuid REFERENCES wiki_type_page_templates(id) ON DELETE SET NULL,
    ADD COLUMN section_label      text,
    ADD COLUMN preview_limit      int  NOT NULL DEFAULT 5,
    ADD CONSTRAINT wiki_page_templates_no_self_parent CHECK (parent_template_id <> id),
    ADD CONSTRAINT wiki_page_templates_preview_limit_nonneg CHECK (preview_limit >= 0);

CREATE INDEX idx_wiki_page_templates_parent
    ON wiki_type_page_templates (parent_template_id)
    WHERE parent_template_id IS NOT NULL;

-- ---------------------------------------------------------------------------
-- Seeds. Topic templates nested under the city cost_of_living "prices"
-- template — one shared type-level group each, auto-assigned to
-- space_type:city. Groups ship as empty schema containers; item fields are
-- added by admins/ops as before (same convention as 000750).
-- ---------------------------------------------------------------------------

INSERT INTO custom_field_groups (name, slug, is_system)
SELECT v.name, v.slug, true
FROM (VALUES
    ('Transportation Prices (city)',  'wiki_city_prices_transportation'),
    ('Restaurant Prices (city)',      'wiki_city_prices_restaurants'),
    ('Food & Groceries Prices (city)','wiki_city_prices_food_groceries'),
    ('Utilities Prices (city)',       'wiki_city_prices_utilities'),
    ('Lifestyle Prices (city)',       'wiki_city_prices_lifestyle')
) AS v(name, slug)
WHERE NOT EXISTS (
    SELECT 1 FROM custom_field_groups g WHERE g.slug = v.slug
);

INSERT INTO custom_field_group_assignments (group_id, entity_class, entity_id, position)
SELECT g.id, 'space_type:' || st.slug, NULL::uuid, 0
FROM custom_field_groups g
JOIN space_types st ON st.slug = 'city'
WHERE g.slug IN (
    'wiki_city_prices_transportation',
    'wiki_city_prices_restaurants',
    'wiki_city_prices_food_groceries',
    'wiki_city_prices_utilities',
    'wiki_city_prices_lifestyle'
)
AND NOT EXISTS (
    SELECT 1 FROM custom_field_group_assignments a
    WHERE a.group_id = g.id AND a.entity_class = 'space_type:' || st.slug AND a.entity_id IS NULL
);

INSERT INTO wiki_type_page_templates
    (category_id, slug, label, field_group_id, parent_template_id, section_label, preview_limit, position, is_system)
SELECT c.id, v.slug, v.label, g.id, p.id, v.section_label, v.preview_limit, v.position, true
FROM wiki_type_categories c
JOIN space_types st ON st.id = c.space_type_id AND st.slug = 'city'
JOIN wiki_type_page_templates p ON p.category_id = c.id AND p.slug = 'prices'
JOIN (VALUES
    ('food-groceries-prices',  'Food & Groceries Prices in {name}',  'Food & Groceries', 5, 1, 'wiki_city_prices_food_groceries'),
    ('restaurant-prices',      'Restaurant Prices in {name}',        'Restaurants',      5, 2, 'wiki_city_prices_restaurants'),
    ('transportation-prices',  'Transportation Prices in {name}',    'Transportation',   5, 3, 'wiki_city_prices_transportation'),
    ('utilities-prices',       'Utilities Prices in {name}',         'Utilities',        4, 4, 'wiki_city_prices_utilities'),
    ('lifestyle-prices',       'Lifestyle Prices in {name}',         'Lifestyle',        5, 5, 'wiki_city_prices_lifestyle')
) AS v(slug, label, section_label, preview_limit, position, group_slug)
    ON true
JOIN custom_field_groups g ON g.slug = v.group_slug
WHERE c.category = 'cost_of_living'
ON CONFLICT (category_id, slug) DO NOTHING;

-- JCE binding: a topic-group comparison type projects its dimensions from
-- wiki_city_prices_transportation directly — "Toronto vs Vancouver
-- transportation" with no config.sections filter.
INSERT INTO comparison_type_definitions (name, slug, description, field_group_id, item_label, is_system)
SELECT 'City Transportation Prices', 'city-transportation-prices',
       'Compare transportation prices between cities',
       g.id, 'City', true
FROM custom_field_groups g
WHERE g.slug = 'wiki_city_prices_transportation'
ON CONFLICT (slug) DO NOTHING;

COMMIT;
