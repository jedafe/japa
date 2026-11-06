-- 001070_comparison_audit_resolution.down.sql
BEGIN;

UPDATE comparison_type_definitions
SET config = config || '{"item_ref_types":["organization","comparison"]}'::jsonb
WHERE slug = 'banks';

UPDATE comparison_type_definitions
SET config = config || '{"item_ref_types":["col_city","comparison"]}'::jsonb
WHERE slug = 'cost_of_living';

INSERT INTO comparison_type_definitions (slug, name, description, field_group_id, config, is_system)
SELECT 'city-cost-of-living', 'City Cost of Living',
    'Compare city cost-of-living data across cities.',
    g.id,
    '{"level":"city","family":"cost_of_living","item_ref_types":["city","comparison"],"max_item_count":5,"default_item_count":3,"allow_nested_comparisons":true}'::jsonb,
    true
FROM custom_field_groups g WHERE g.slug = 'wiki_city_cost_of_living'
ON CONFLICT (slug) DO NOTHING;

DELETE FROM comparison_type_definitions
WHERE slug IN ('remittance', 'language-tests', 'travel-routes');

UPDATE custom_field_values SET entity_type = 'program', updated_at = now()
FROM custom_fields f
JOIN custom_field_groups g ON g.id = f.group_id
WHERE custom_field_values.field_id = f.id
  AND g.slug LIKE 'wiki_program_%'
  AND custom_field_values.entity_type = 'course'
  AND NOT EXISTS (
      SELECT 1 FROM custom_field_values c
      WHERE c.field_id = custom_field_values.field_id
        AND c.entity_type = 'program' AND c.entity_id = custom_field_values.entity_id
  );

DELETE FROM custom_field_group_assignments a
USING custom_field_groups g
WHERE a.group_id = g.id AND g.slug IN ('wiki_remittance_overview','wiki_languagetest_overview','wiki_travel_overview');

DELETE FROM custom_fields f
USING custom_field_groups g
WHERE f.group_id = g.id AND g.slug IN ('wiki_remittance_overview','wiki_languagetest_overview','wiki_travel_overview');

DELETE FROM custom_field_groups
WHERE slug IN ('wiki_remittance_overview','wiki_languagetest_overview','wiki_travel_overview');

COMMIT;
