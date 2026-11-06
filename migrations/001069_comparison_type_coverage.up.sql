-- 001069_comparison_type_coverage.up.sql
-- Comparison-type coverage audit (§30 Sub-spec 5):
--
--  1. Delete dead types 'city-prices' + 'city-transportation-prices' —
--     empty item_ref_types (no items could ever be added), zero dims, and
--     their groups are already covered by city-cost-of-living /
--     col-transportation.
--
--  2. Create types for resolver-backed entities that have a canonical
--     cfv surface but no type:
--       embassies      → wiki_embassy_overview     (resolver 'embassy',
--                        cfv bucket via 'embassy'/'space_type:embassy')
--       neighborhoods  → wiki_neighborhood_overview (resolver 'neighborhood',
--                        space-backed 'space' bucket)
--       job-listings   → job_listing_fields         (resolver 'job_listing',
--                        'content' bucket via 'content' class assignment)
--       housing        → guide_accommodation_fields (resolver 'housing_guide',
--                        'content' bucket — same convention as scholarships)
--
--    Config shape mirrors the visa-routes/cities templates.
--
--    NOT created (flagged): programs (no 'program' resolver), remittance,
--    languagetest, travel (no dedicated field group / unclear canonical
--    surface), community_org, stay, guide (no resolvers).

BEGIN;

DELETE FROM comparison_type_definitions
WHERE slug IN ('city-prices', 'city-transportation-prices')
  AND NOT EXISTS (
      SELECT 1 FROM comparison_dimensions d
      WHERE d.type_id = comparison_type_definitions.id
        AND d.deleted_at IS NULL
  );

INSERT INTO comparison_type_definitions (slug, name, description, field_group_id, config, is_system)
SELECT 'embassies', 'Embassies',
    'Compare embassies and consulates — services, jurisdiction, appointment and contact details.',
    g.id,
    '{"level":"embassy","family":"embassies","item_ref_types":["embassy","comparison"],"max_item_count":5,"default_item_count":3,"allowed_nested_types":["countries","cities"],"allow_nested_comparisons":true}'::jsonb,
    true
FROM custom_field_groups g WHERE g.slug = 'wiki_embassy_overview';

INSERT INTO comparison_type_definitions (slug, name, description, field_group_id, config, is_system)
SELECT 'neighborhoods', 'Neighborhoods',
    'Compare neighborhoods — safety, amenities, transport, schools and demographics.',
    g.id,
    '{"level":"neighborhood","family":"geo","item_ref_types":["neighborhood","comparison"],"max_item_count":5,"default_item_count":3,"allowed_nested_types":["cities","cost_of_living"],"allow_nested_comparisons":true}'::jsonb,
    true
FROM custom_field_groups g WHERE g.slug = 'wiki_neighborhood_overview';

INSERT INTO comparison_type_definitions (slug, name, description, field_group_id, config, is_system)
SELECT 'job-listings', 'Job Listings',
    'Compare job listings — role, salary, location, requirements and application details.',
    g.id,
    '{"level":"job","family":"marketplace","item_ref_types":["job_listing","comparison"],"max_item_count":5,"default_item_count":3,"allowed_nested_types":["cities","cost_of_living"],"allow_nested_comparisons":true}'::jsonb,
    true
FROM custom_field_groups g WHERE g.slug = 'job_listing_fields';

INSERT INTO comparison_type_definitions (slug, name, description, field_group_id, config, is_system)
SELECT 'housing', 'Housing & Accommodation',
    'Compare housing and accommodation guides — rent, deposits, facilities and lease terms.',
    g.id,
    '{"level":"housing","family":"guides","item_ref_types":["housing_guide","comparison"],"max_item_count":5,"default_item_count":3,"allowed_nested_types":["cities","cost_of_living"],"allow_nested_comparisons":true}'::jsonb,
    true
FROM custom_field_groups g WHERE g.slug = 'guide_accommodation_fields';

COMMIT;
