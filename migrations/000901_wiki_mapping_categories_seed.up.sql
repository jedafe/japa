-- 000901_wiki_mapping_categories_seed.up.sql
-- JCE ↔ Wiki field-group mapping, phase M3 (category registry seeds):
-- spec docs/superpowers/specs/2026-10-20-jce-wiki-field-group-mapping-design.md
--
-- Three seed sets, all following the 000739 convention — every category owns
-- a dedicated group named wiki_{type_slug}_{category}:
--
--   a) Categories for the new wiki types from 000900 — neighborhood, visa
--      route + 6 kinds (kinds inherit the visa-route template with per-kind
--      groups), course, scholarship, bank, embassy.
--   b) Three new city cost_of_living topic pages (subpages): rent & housing,
--      healthcare, childcare & school fees — each with its own group.
--   c) Geo/university category expansion to the mindmap's canonical sets.
--
-- Entity-bound types' groups assign to the entity class ('visa_route',
-- 'organization', …) so cfv values land on entity rows — the same store JCE
-- resolvers already merge. Kind-scoping discipline is editorial: a
-- student_visa group's fields are only authored against student routes.

BEGIN;

-- ===========================================================================
-- a) Categories for the new wiki types.
-- ===========================================================================
CREATE TEMP TABLE _new_type_cat_seed (
    type_slug text,
    slug      text,
    category  text,
    label     text,
    position  int
) ON COMMIT DROP;

INSERT INTO _new_type_cat_seed (type_slug, slug, category, label, position) VALUES
    -- neighborhood
    ('neighborhood', 'overview',    'overview',    'Overview',    1),
    ('neighborhood', 'safety',      'safety',      'Safety',      2),
    ('neighborhood', 'rent-prices', 'rent_prices', 'Rent Prices', 3),
    ('neighborhood', 'amenities',   'amenities',   'Amenities',   4),
    -- visa route (base kind) — kinds inherit this exact template
    ('visa_route', 'overview',            'overview',            'Overview',                  1),
    ('visa_route', 'requirements',        'requirements',        'Requirements & Eligibility', 2),
    ('visa_route', 'fees-costs',          'fees_costs',          'Fees & Costs',               3),
    ('visa_route', 'processing-time',     'processing_time',     'Processing Time',            4),
    ('visa_route', 'application-process', 'application_process', 'Application Process',        5),
    -- visa kinds (inherited template → per-kind groups)
    ('student_visa', 'overview',            'overview',            'Overview',                  1),
    ('student_visa', 'requirements',        'requirements',        'Requirements & Eligibility', 2),
    ('student_visa', 'fees-costs',          'fees_costs',          'Fees & Costs',               3),
    ('student_visa', 'processing-time',     'processing_time',     'Processing Time',            4),
    ('student_visa', 'application-process', 'application_process', 'Application Process',        5),
    ('work_visa', 'overview',            'overview',            'Overview',                  1),
    ('work_visa', 'requirements',        'requirements',        'Requirements & Eligibility', 2),
    ('work_visa', 'fees-costs',          'fees_costs',          'Fees & Costs',               3),
    ('work_visa', 'processing-time',     'processing_time',     'Processing Time',            4),
    ('work_visa', 'application-process', 'application_process', 'Application Process',        5),
    ('family_visa', 'overview',            'overview',            'Overview',                  1),
    ('family_visa', 'requirements',        'requirements',        'Requirements & Eligibility', 2),
    ('family_visa', 'fees-costs',          'fees_costs',          'Fees & Costs',               3),
    ('family_visa', 'processing-time',     'processing_time',     'Processing Time',            4),
    ('family_visa', 'application-process', 'application_process', 'Application Process',        5),
    ('tourist_visa', 'overview',            'overview',            'Overview',                  1),
    ('tourist_visa', 'requirements',        'requirements',        'Requirements & Eligibility', 2),
    ('tourist_visa', 'fees-costs',          'fees_costs',          'Fees & Costs',               3),
    ('tourist_visa', 'processing-time',     'processing_time',     'Processing Time',            4),
    ('tourist_visa', 'application-process', 'application_process', 'Application Process',        5),
    ('business_visa', 'overview',            'overview',            'Overview',                  1),
    ('business_visa', 'requirements',        'requirements',        'Requirements & Eligibility', 2),
    ('business_visa', 'fees-costs',          'fees_costs',          'Fees & Costs',               3),
    ('business_visa', 'processing-time',     'processing_time',     'Processing Time',            4),
    ('business_visa', 'application-process', 'application_process', 'Application Process',        5),
    ('pr_pathway', 'overview',            'overview',            'Overview',                  1),
    ('pr_pathway', 'requirements',        'requirements',        'Requirements & Eligibility', 2),
    ('pr_pathway', 'fees-costs',          'fees_costs',          'Fees & Costs',               3),
    ('pr_pathway', 'processing-time',     'processing_time',     'Processing Time',            4),
    ('pr_pathway', 'application-process', 'application_process', 'Application Process',        5),
    -- course
    ('course', 'overview',           'overview',           'Overview',           1),
    ('course', 'entry-requirements', 'entry_requirements', 'Entry Requirements', 2),
    ('course', 'tuition-fees',       'tuition_fees',       'Tuition & Fees',     3),
    ('course', 'career-outcomes',    'career_outcomes',    'Career Outcomes',    4),
    -- scholarship
    ('scholarship', 'overview',                'overview',                'Overview',                  1),
    ('scholarship', 'eligibility',             'eligibility',             'Eligibility',                2),
    ('scholarship', 'coverage-benefits',       'coverage_benefits',       'Coverage & Benefits',        3),
    ('scholarship', 'application-deadlines',   'application_deadlines',   'Application & Deadlines',    4),
    -- bank
    ('bank', 'overview',      'overview',      'Overview',                    1),
    ('bank', 'accounts-fees', 'accounts_fees', 'Account Types & Fees',        2),
    ('bank', 'requirements',  'requirements',  'Requirements (Non-Resident)', 3),
    ('bank', 'remittance-fx', 'remittance_fx', 'Remittance & FX',             4),
    ('bank', 'coverage',      'coverage',      'Branch & ATM Coverage',       5),
    -- embassy
    ('embassy', 'overview',             'overview',             'Overview',             1),
    ('embassy', 'consular-services',    'consular_services',    'Consular Services',    2),
    ('embassy', 'appointment-booking',  'appointment_booking',  'Appointment Booking',  3),
    ('embassy', 'contact-location',     'contact_location',     'Contact & Location',   4);

-- Dedicated per-type groups: wiki_{type_slug}_{category}.
INSERT INTO custom_field_groups (name, slug, is_system)
SELECT c.label || ' (' || c.type_slug || ')',
       'wiki_' || c.type_slug || '_' || c.category,
       true
FROM _new_type_cat_seed c
ON CONFLICT (slug) DO NOTHING;

INSERT INTO wiki_type_categories (id, space_type_id, slug, category, label, field_group_id, position, is_system)
SELECT gen_random_uuid(), st.id, c.slug, c.category, c.label, g.id, c.position, true
FROM _new_type_cat_seed c
JOIN space_types st ON st.slug = c.type_slug
JOIN custom_field_groups g ON g.slug = 'wiki_' || c.type_slug || '_' || c.category
ON CONFLICT (space_type_id, slug) DO NOTHING;

-- Group assignments: entity-bound types → their entity class (cfv targets
-- entity rows); space-bound neighborhood → its space_type class.
INSERT INTO custom_field_group_assignments (group_id, entity_class, entity_id, position)
SELECT DISTINCT c.field_group_id, 'space_type:neighborhood', NULL::uuid, 0
FROM wiki_type_categories c
JOIN space_types st ON st.id = c.space_type_id AND st.slug = 'neighborhood'
WHERE NOT EXISTS (
    SELECT 1 FROM custom_field_group_assignments a
    WHERE a.group_id = c.field_group_id AND a.entity_class = 'space_type:neighborhood' AND a.entity_id IS NULL
);

INSERT INTO custom_field_group_assignments (group_id, entity_class, entity_id, position)
SELECT DISTINCT c.field_group_id, cfg.entity_kind, NULL::uuid, 0
FROM wiki_type_categories c
JOIN space_types st ON st.id = c.space_type_id
JOIN wiki_type_configs cfg ON cfg.space_type_id = c.space_type_id
WHERE cfg.entity_kind IS NOT NULL
  AND st.slug IN ('visa_route','student_visa','work_visa','family_visa','tourist_visa',
                  'business_visa','pr_pathway','course','scholarship','bank','embassy')
  AND NOT EXISTS (
    SELECT 1 FROM custom_field_group_assignments a
    WHERE a.group_id = c.field_group_id AND a.entity_class = cfg.entity_kind AND a.entity_id IS NULL
);

-- ===========================================================================
-- b) Three new city cost_of_living topic pages (subpages, own groups).
-- ===========================================================================
INSERT INTO custom_field_groups (name, slug, is_system)
SELECT v.name, v.slug, true
FROM (VALUES
    ('Rent & Housing Prices (city)',   'wiki_city_prices_rent_housing'),
    ('Healthcare Prices (city)',       'wiki_city_prices_healthcare'),
    ('Childcare & School Fees (city)', 'wiki_city_prices_childcare')
) AS v(name, slug)
WHERE NOT EXISTS (SELECT 1 FROM custom_field_groups g WHERE g.slug = v.slug);

INSERT INTO custom_field_group_assignments (group_id, entity_class, entity_id, position)
SELECT g.id, 'space_type:city', NULL::uuid, 0
FROM custom_field_groups g
WHERE g.slug IN ('wiki_city_prices_rent_housing','wiki_city_prices_healthcare','wiki_city_prices_childcare')
AND NOT EXISTS (
    SELECT 1 FROM custom_field_group_assignments a
    WHERE a.group_id = g.id AND a.entity_class = 'space_type:city' AND a.entity_id IS NULL
);

INSERT INTO wiki_type_page_templates
    (category_id, slug, label, field_group_id, parent_template_id, section_label, preview_limit, position, is_system)
SELECT c.id, v.slug, v.label, g.id, NULL, v.section_label, v.preview_limit, v.position, true
FROM wiki_type_categories c
JOIN space_types st ON st.id = c.space_type_id AND st.slug = 'city'
JOIN (VALUES
    ('rent-housing-prices',  'Rent & Housing Prices in {name}',  'Rent & Housing',  5, 6, 'wiki_city_prices_rent_housing'),
    ('healthcare-prices',    'Healthcare Prices in {name}',      'Healthcare',      5, 7, 'wiki_city_prices_healthcare'),
    ('childcare-school-fees','Childcare & School Fees in {name}','Childcare & School',5, 8, 'wiki_city_prices_childcare')
) AS v(slug, label, section_label, preview_limit, position, group_slug) ON true
JOIN custom_field_groups g ON g.slug = v.group_slug
WHERE c.category = 'cost_of_living'
ON CONFLICT (category_id, slug) DO NOTHING;

-- Register the new subpages in the city COL page-scope rollup (000857).
-- Guarded on the pages key existing — NULL || array would wipe the list.
UPDATE wiki_type_categories c
SET rollup = jsonb_set(rollup, '{pages}',
    (rollup->'pages') || '["rent-housing-prices","healthcare-prices","childcare-school-fees"]'::jsonb)
FROM space_types st
WHERE c.space_type_id = st.id
  AND st.slug = 'city'
  AND c.category = 'cost_of_living'
  AND rollup->'pages' IS NOT NULL;

-- ===========================================================================
-- c) Geo/university category expansion to the mindmap's canonical sets.
--    Positions continue past each type's existing max.
-- ===========================================================================
CREATE TEMP TABLE _geo_cat_seed (
    type_slug text,
    slug      text,
    category  text,
    label     text,
    position  int
) ON COMMIT DROP;

INSERT INTO _geo_cat_seed (type_slug, slug, category, label, position) VALUES
    -- country (existing max 8; attractions took 9 in 000795)
    ('country', 'safety',  'safety',  'Safety & Crime',      10),
    ('country', 'jobs',    'jobs',    'Jobs & Economy',      11),
    ('country', 'taxes',   'taxes',   'Taxes',               12),
    ('country', 'weather', 'weather', 'Weather & Climate',   13),
    -- state (existing max 6; attractions took 7)
    ('state', 'safety',     'safety',     'Safety & Crime',     8),
    ('state', 'jobs',       'jobs',       'Jobs & Economy',    9),
    ('state', 'schools',    'schools',    'Schools & Education',10),
    ('state', 'taxes',      'taxes',      'Taxes',             11),
    ('state', 'weather',    'weather',    'Weather & Climate', 12),
    ('state', 'banking',    'banking',    'Banking',           13),
    -- city (existing max 6; attractions took 7)
    ('city', 'safety',         'safety',         'Safety & Crime',          8),
    ('city', 'jobs',           'jobs',           'Jobs & Salaries',         9),
    ('city', 'schools',        'schools',        'Schools & Education',     10),
    ('city', 'transportation', 'transportation', 'Transportation & Commute',11),
    ('city', 'utilities',      'utilities',      'Utilities & Internet',    12),
    ('city', 'weather',        'weather',        'Weather & Climate',       13),
    ('city', 'banking',        'banking',        'Banking',                 14),
    -- university (existing max 9)
    ('university', 'rankings', 'rankings', 'Rankings & Reputation', 10);

INSERT INTO custom_field_groups (name, slug, is_system)
SELECT c.label || ' (' || c.type_slug || ')',
       'wiki_' || c.type_slug || '_' || c.category,
       true
FROM _geo_cat_seed c
ON CONFLICT (slug) DO NOTHING;

INSERT INTO wiki_type_categories (id, space_type_id, slug, category, label, field_group_id, position, is_system)
SELECT gen_random_uuid(), st.id, c.slug, c.category, c.label, g.id, c.position, true
FROM _geo_cat_seed c
JOIN space_types st ON st.slug = c.type_slug
JOIN custom_field_groups g ON g.slug = 'wiki_' || c.type_slug || '_' || c.category
ON CONFLICT (space_type_id, slug) DO NOTHING;

INSERT INTO custom_field_group_assignments (group_id, entity_class, entity_id, position)
SELECT DISTINCT c.field_group_id, 'space_type:' || st.slug, NULL::uuid, 0
FROM wiki_type_categories c
JOIN space_types st ON st.id = c.space_type_id
JOIN _geo_cat_seed s ON s.type_slug = st.slug AND s.slug = c.slug
WHERE NOT EXISTS (
    SELECT 1 FROM custom_field_group_assignments a
    WHERE a.group_id = c.field_group_id
      AND a.entity_class = 'space_type:' || st.slug
      AND a.entity_id IS NULL
);

COMMIT;
