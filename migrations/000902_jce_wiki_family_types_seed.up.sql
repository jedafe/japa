-- 000902_jce_wiki_family_types_seed.up.sql
-- JCE ↔ Wiki field-group mapping, phase M4b (family type seeds):
-- spec docs/superpowers/specs/2026-10-20-jce-wiki-field-group-mapping-design.md
--
-- Second batch of wiki-bound comparison types, seeded where both the wiki
-- group (000901) and the JCE resolver exist:
--
--   * education family   → scholarships → wiki_scholarship_overview
--                          (item_ref 'scholarship' resolver exists)
--   * visa_routes family → visa-route + 6 kinds → wiki_{kind}_overview
--                          (item_ref 'visa_route' resolver exists; kinds are
--                           marked via config.subtype — kind filtering in the
--                           item picker is frontend M6 work)
--
-- Deliberately deferred: neighborhoods (no 'neighborhood' item_ref resolver
-- yet — space-backed ref types don't include it), embassies (no 'embassy'
-- resolver), bank-overview rebind (existing 'banks' type keeps
-- organization_fields until groups carry fields — spec open question #3).

BEGIN;

INSERT INTO comparison_type_definitions (name, slug, description, field_group_id, item_label, is_system, config)
SELECT v.name, v.slug, v.description, g.id, v.item_label, true, v.config::jsonb
FROM (VALUES
    ('Scholarships', 'scholarships',
     'Compare scholarships side by side', 'Scholarship',
     'wiki_scholarship_overview',
     '{"item_ref_types": ["scholarship", "comparison"], "allow_nested_comparisons": true, "allowed_nested_types": ["universities", "courses", "countries", "states", "cities"], "max_item_count": 5, "default_item_count": 3, "family": "education", "level": "scholarship"}'),
    ('Visa Routes', 'visa-routes',
     'Compare visa routes between countries', 'Visa Route',
     'wiki_visa_route_overview',
     '{"item_ref_types": ["visa_route", "comparison"], "allow_nested_comparisons": true, "allowed_nested_types": ["universities", "countries", "states", "cities"], "max_item_count": 5, "default_item_count": 3, "family": "visa_routes", "level": "visa_route"}'),
    ('Student Visas', 'student-visas',
     'Compare student visa routes', 'Student Visa',
     'wiki_student_visa_overview',
     '{"item_ref_types": ["visa_route", "comparison"], "allow_nested_comparisons": true, "allowed_nested_types": ["universities", "countries", "states", "cities"], "max_item_count": 5, "default_item_count": 3, "family": "visa_routes", "level": "student_visa", "subtype": "student"}'),
    ('Work Visas', 'work-visas',
     'Compare work visa routes', 'Work Visa',
     'wiki_work_visa_overview',
     '{"item_ref_types": ["visa_route", "comparison"], "allow_nested_comparisons": true, "allowed_nested_types": ["universities", "countries", "states", "cities"], "max_item_count": 5, "default_item_count": 3, "family": "visa_routes", "level": "work_visa", "subtype": "work"}'),
    ('Family Visas', 'family-visas',
     'Compare family reunification visa routes', 'Family Visa',
     'wiki_family_visa_overview',
     '{"item_ref_types": ["visa_route", "comparison"], "allow_nested_comparisons": true, "allowed_nested_types": ["universities", "countries", "states", "cities"], "max_item_count": 5, "default_item_count": 3, "family": "visa_routes", "level": "family_visa", "subtype": "family"}'),
    ('Tourist Visas', 'tourist-visas',
     'Compare tourist and visitor visa routes', 'Tourist Visa',
     'wiki_tourist_visa_overview',
     '{"item_ref_types": ["visa_route", "comparison"], "allow_nested_comparisons": true, "allowed_nested_types": ["universities", "countries", "states", "cities"], "max_item_count": 5, "default_item_count": 3, "family": "visa_routes", "level": "tourist_visa", "subtype": "tourist"}'),
    ('Business Visas', 'business-visas',
     'Compare business and investor visa routes', 'Business Visa',
     'wiki_business_visa_overview',
     '{"item_ref_types": ["visa_route", "comparison"], "allow_nested_comparisons": true, "allowed_nested_types": ["universities", "countries", "states", "cities"], "max_item_count": 5, "default_item_count": 3, "family": "visa_routes", "level": "business_visa", "subtype": "business"}'),
    ('PR Pathways', 'pr-pathways',
     'Compare permanent residency pathways', 'PR Pathway',
     'wiki_pr_pathway_overview',
     '{"item_ref_types": ["visa_route", "comparison"], "allow_nested_comparisons": true, "allowed_nested_types": ["universities", "countries", "states", "cities"], "max_item_count": 5, "default_item_count": 3, "family": "visa_routes", "level": "pr_pathway", "subtype": "pr"}')
) AS v(name, slug, description, item_label, group_slug, config)
JOIN custom_field_groups g ON g.slug = v.group_slug
ON CONFLICT (slug) DO NOTHING;

COMMIT;
