-- 000546_jce_geo_and_nested_comparison_types.up.sql
-- Adds country/state/city system comparison types that resolve to the matching
-- geo spaces, enables nested comparison items across all system comparison
-- types, and seeds the config values required by the JCE item picker.
--
-- Ref: docs/superpowers/specs/2026-08-22-jce-subspec-1-design.md

BEGIN;

-- Create a shared geo field group and a few default dimensions.
INSERT INTO custom_field_groups (tenant_id, name, slug, description, is_system)
VALUES (
    '00000000-0000-0000-0000-000000000001',
    'Geo Fields',
    'geo_fields',
    'Default dimensions for country, state, and city comparisons',
    true
)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO custom_fields (group_id, tenant_id, label, key, field_type, position, is_system, show_in_frontend)
SELECT cfg.id,
       '00000000-0000-0000-0000-000000000001',
       label,
       key,
       'text',
       position,
       true,
       true
FROM custom_field_groups cfg
CROSS JOIN (VALUES
    ('Population', 'population', 0),
    ('Area', 'area', 1),
    ('Capital/Centre', 'capital', 2),
    ('Cost of Living', 'cost_of_living', 3),
    ('Language', 'language', 4)
) AS fields(label, key, position)
WHERE cfg.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND cfg.slug = 'geo_fields'
ON CONFLICT (group_id, key) DO NOTHING;

-- Insert country/state/city system comparison types.
INSERT INTO comparison_type_definitions (id, tenant_id, name, slug, description, field_group_id, item_label, is_system, config)
SELECT
    gen_random_uuid(),
    '00000000-0000-0000-0000-000000000001',
    type.name,
    type.slug,
    type.description,
    cfg.id,
    type.item_label,
    true,
    type.config
FROM custom_field_groups cfg
CROSS JOIN (VALUES
    ('Countries', 'countries', 'Compare countries across key dimensions', 'Country',
     '{"item_ref_types": ["country", "comparison"], "allow_nested_comparisons": true, "allowed_nested_types": ["universities", "cost_of_living", "banks", "courses", "visa-types", "mentors", "states", "cities"], "max_item_count": 5, "default_item_count": 3}'::jsonb),
    ('States', 'states', 'Compare states/provinces across key dimensions', 'State',
     '{"item_ref_types": ["state", "comparison"], "allow_nested_comparisons": true, "allowed_nested_types": ["universities", "cost_of_living", "banks", "courses", "visa-types", "mentors", "countries", "cities"], "max_item_count": 5, "default_item_count": 3}'::jsonb),
    ('Cities', 'cities', 'Compare cities across key dimensions', 'City',
     '{"item_ref_types": ["city", "comparison"], "allow_nested_comparisons": true, "allowed_nested_types": ["universities", "cost_of_living", "banks", "courses", "visa-types", "mentors", "countries", "states"], "max_item_count": 5, "default_item_count": 3}'::jsonb)
) AS type(name, slug, description, item_label, config)
WHERE cfg.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND cfg.slug = 'geo_fields'
ON CONFLICT (tenant_id, slug) DO NOTHING;

-- Enable nested comparison items for all existing system comparison types
-- and append the comparison resolver to their allowed item_ref_types.
UPDATE comparison_type_definitions
SET config = jsonb_strip_nulls(
    COALESCE(config, '{}'::jsonb)
    || jsonb_build_object('allow_nested_comparisons', true)
    || jsonb_build_object('allowed_nested_types',
        ARRAY['universities', 'cost_of_living', 'banks', 'courses', 'visa-types', 'mentors', 'countries', 'states', 'cities']
    )
    || CASE
        WHEN slug = 'universities' THEN '{"item_ref_types": ["university", "comparison"]}'::jsonb
        WHEN slug = 'cost_of_living' THEN '{"item_ref_types": ["col_city", "comparison"]}'::jsonb
        WHEN slug = 'banks' THEN '{"item_ref_types": ["organization", "comparison"]}'::jsonb
        WHEN slug = 'courses' THEN '{"item_ref_types": ["course", "comparison"]}'::jsonb
        WHEN slug = 'visa-types' THEN '{"item_ref_types": ["visa_route", "comparison"]}'::jsonb
        WHEN slug = 'mentors' THEN '{"item_ref_types": ["mentor", "comparison"]}'::jsonb
        ELSE '{}'::jsonb
       END
)
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND is_system = true;

COMMIT;
