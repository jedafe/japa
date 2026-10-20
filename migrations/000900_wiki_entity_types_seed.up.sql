-- 000900_wiki_entity_types_seed.up.sql
-- JCE ↔ Wiki field-group mapping, phase M2 (type registry seeds):
-- spec docs/superpowers/specs/2026-10-20-jce-wiki-field-group-mapping-design.md
--
-- Registers the entity kinds wiki pages may bind, then seeds the mindmap's
-- new wiki types:
--
--   * neighborhood      — space-bound, child of city (geo chain tail)
--   * visa_route        — entity-bound (visa_routes), child of country
--   * student/work/family/tourist/business visas + pr_pathway
--                       — child wiki types over visa_routes via
--                         entity_discriminator on visa_category
--   * course            — entity-bound (university_course_offerings), child of university
--   * scholarship       — entity-bound (scholarships), child of university|country
--   * bank              — entity-bound (organizations where org_type='bank'),
--                         child of country
--   * embassy           — entity-bound (monitored_embassies), child of country
--
-- Entity-bound types get wiki_only space_types rows purely as type keys —
-- no spaces are created for them (attraction pattern, one step further).

BEGIN;

-- ---------------------------------------------------------------------------
-- Entity kinds registry.
-- scholarships has no slug column yet — slug_column NULL (open question in
-- spec; add a slug column when scholarship pages get public URLs).
-- ---------------------------------------------------------------------------
INSERT INTO wiki_entity_kinds (entity_kind, source_table, id_column, slug_column, label) VALUES
    ('visa_route',   'visa_routes',                 'id', 'slug', 'Visa Route'),
    ('organization', 'organizations',               'id', 'slug', 'Organization'),
    ('course',       'university_course_offerings', 'id', 'slug', 'Course Offering'),
    ('scholarship',  'scholarships',                'id', NULL,   'Scholarship'),
    ('embassy',      'monitored_embassies',         'id', 'slug', 'Embassy')
ON CONFLICT (entity_kind) DO NOTHING;

-- ---------------------------------------------------------------------------
-- Type keys. Entity-bound types are wiki_only — they never host spaces.
-- ---------------------------------------------------------------------------
INSERT INTO space_types (id, name, slug, description, icon, field_group_id, is_system, category, wiki_only) VALUES
    ('00000000-0000-0000-0000-000000000072', 'Neighborhood',  'neighborhood',  'Neighborhood or district within a city', 'map-pin', NULL, true, 'geo', true),
    ('00000000-0000-0000-0000-000000000073', 'Visa Route',    'visa_route',    'Visa or immigration pathway (entity-bound wiki type)', 'plane', NULL, true, NULL, true),
    ('00000000-0000-0000-0000-000000000074', 'Student Visa',  'student_visa',  'Student visa route kind', 'graduation-cap', NULL, true, NULL, true),
    ('00000000-0000-0000-0000-000000000075', 'Work Visa',     'work_visa',     'Work visa route kind', 'briefcase', NULL, true, NULL, true),
    ('00000000-0000-0000-0000-000000000076', 'Family Visa',   'family_visa',   'Family reunification visa route kind', 'users', NULL, true, NULL, true),
    ('00000000-0000-0000-0000-000000000077', 'Tourist Visa',  'tourist_visa',  'Tourist/visitor visa route kind', 'camera', NULL, true, NULL, true),
    ('00000000-0000-0000-0000-000000000078', 'Business Visa', 'business_visa', 'Business/investor visa route kind', 'landmark', NULL, true, NULL, true),
    ('00000000-0000-0000-0000-000000000079', 'PR Pathway',    'pr_pathway',    'Permanent residency pathway kind', 'flag', NULL, true, NULL, true),
    ('00000000-0000-0000-0000-00000000007a', 'Course',        'course',        'University course offering (entity-bound wiki type)', 'book-open', NULL, true, NULL, true),
    ('00000000-0000-0000-0000-00000000007b', 'Scholarship',   'scholarship',   'Scholarship (entity-bound wiki type)', 'award', NULL, true, NULL, true),
    ('00000000-0000-0000-0000-00000000007c', 'Bank',          'bank',          'Bank organization (entity-bound wiki type)', 'landmark', NULL, true, NULL, true),
    ('00000000-0000-0000-0000-00000000007d', 'Embassy',       'embassy',       'Embassy (entity-bound wiki type)', 'building', NULL, true, NULL, true)
ON CONFLICT (id) DO NOTHING;

-- ---------------------------------------------------------------------------
-- Wiki type configs. entity_kind NULL = space-bound; set = entity-bound with
-- optional {column: value} discriminator on the source table.
-- Positions continue from attraction (5).
-- ---------------------------------------------------------------------------
INSERT INTO wiki_type_configs (space_type_id, enabled, label, icon, seed_stubs, position, entity_kind, entity_discriminator) VALUES
    ('00000000-0000-0000-0000-000000000072', true, NULL, NULL, false, 6,  NULL,           NULL),
    ('00000000-0000-0000-0000-000000000073', true, NULL, NULL, false, 7,  'visa_route',   NULL),
    ('00000000-0000-0000-0000-000000000074', true, NULL, NULL, false, 8,  'visa_route',   '{"visa_category":"student"}'::jsonb),
    ('00000000-0000-0000-0000-000000000075', true, NULL, NULL, false, 9,  'visa_route',   '{"visa_category":"work"}'::jsonb),
    ('00000000-0000-0000-0000-000000000076', true, NULL, NULL, false, 10, 'visa_route',   '{"visa_category":"family"}'::jsonb),
    ('00000000-0000-0000-0000-000000000077', true, NULL, NULL, false, 11, 'visa_route',   '{"visa_category":"tourist"}'::jsonb),
    ('00000000-0000-0000-0000-000000000078', true, NULL, NULL, false, 12, 'visa_route',   '{"visa_category":"business"}'::jsonb),
    ('00000000-0000-0000-0000-000000000079', true, NULL, NULL, false, 13, 'visa_route',   '{"visa_category":"pr"}'::jsonb),
    ('00000000-0000-0000-0000-00000000007a', true, NULL, NULL, false, 14, 'course',       NULL),
    ('00000000-0000-0000-0000-00000000007b', true, NULL, NULL, false, 15, 'scholarship',  NULL),
    ('00000000-0000-0000-0000-00000000007c', true, NULL, NULL, false, 16, 'organization', '{"org_type":"bank"}'::jsonb),
    ('00000000-0000-0000-0000-00000000007d', true, NULL, NULL, false, 17, 'embassy',      NULL)
ON CONFLICT (space_type_id) DO NOTHING;

-- ---------------------------------------------------------------------------
-- Type edges: geo chain tail + entity types under their hosts.
-- ---------------------------------------------------------------------------
INSERT INTO wiki_type_parents (space_type_id, parent_space_type_id) VALUES
    -- neighborhood → city
    ('00000000-0000-0000-0000-000000000072', '00000000-0000-0000-0000-000000000062'),
    -- visa route → country
    ('00000000-0000-0000-0000-000000000073', '00000000-0000-0000-0000-000000000060'),
    -- visa kinds → visa route
    ('00000000-0000-0000-0000-000000000074', '00000000-0000-0000-0000-000000000073'),
    ('00000000-0000-0000-0000-000000000075', '00000000-0000-0000-0000-000000000073'),
    ('00000000-0000-0000-0000-000000000076', '00000000-0000-0000-0000-000000000073'),
    ('00000000-0000-0000-0000-000000000077', '00000000-0000-0000-0000-000000000073'),
    ('00000000-0000-0000-0000-000000000078', '00000000-0000-0000-0000-000000000073'),
    ('00000000-0000-0000-0000-000000000079', '00000000-0000-0000-0000-000000000073'),
    -- course → university
    ('00000000-0000-0000-0000-00000000007a', '00000000-0000-0000-0000-000000000064'),
    -- scholarship → university + country ("any or all")
    ('00000000-0000-0000-0000-00000000007b', '00000000-0000-0000-0000-000000000064'),
    ('00000000-0000-0000-0000-00000000007b', '00000000-0000-0000-0000-000000000060'),
    -- bank → country
    ('00000000-0000-0000-0000-00000000007c', '00000000-0000-0000-0000-000000000060'),
    -- embassy → country
    ('00000000-0000-0000-0000-00000000007d', '00000000-0000-0000-0000-000000000060')
ON CONFLICT DO NOTHING;

COMMIT;
