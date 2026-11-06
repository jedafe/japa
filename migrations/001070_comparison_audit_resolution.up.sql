-- 001070_comparison_audit_resolution.up.sql
-- Final audit resolution:
--   1. 'banks': item_ref_types 'organization' → 'bank' (the wiki entity
--      resolver; org banks aren't comparable).
--   2. Unify 'city-cost-of-living' into 'cost_of_living' — same group, same
--      dims; cost_of_living gains 'city' item refs, the duplicate type is
--      deleted (zero dims existed).
--   3. 'program' cfv rows → 'course' bucket (ValueEntityType normalization;
--      programs are course offerings).
--   4. Create overview groups + assignments + types for the three resolver-
--      backed entities lacking comparison coverage: remittance,
--      languagetest, travel.

BEGIN;

UPDATE comparison_type_definitions
SET config = config || '{"item_ref_types":["bank","comparison"]}'::jsonb
WHERE slug = 'banks';

UPDATE comparison_type_definitions
SET config = config || '{"item_ref_types":["col_city","city","comparison"]}'::jsonb
WHERE slug = 'cost_of_living';

DELETE FROM comparison_type_definitions WHERE slug = 'city-cost-of-living';

UPDATE custom_field_values SET entity_type = 'course', updated_at = now()
WHERE entity_type = 'program'
  AND NOT EXISTS (
      SELECT 1 FROM custom_field_values c
      WHERE c.field_id = custom_field_values.field_id
        AND c.entity_type = 'course' AND c.entity_id = custom_field_values.entity_id
  );
DELETE FROM custom_field_values WHERE entity_type = 'program';

-- New overview groups (mirroring the wiki_* pattern) + 'scholarship'-style
-- entity-class assignments for the jce_entity token.
INSERT INTO custom_field_groups (slug, name, description, is_system) VALUES
    ('wiki_remittance_overview',   'Overview (remittance)',   'Overview fields for remittance service pages.', true),
    ('wiki_languagetest_overview', 'Overview (language test)','Overview fields for language test pages.',      true),
    ('wiki_travel_overview',       'Overview (travel)',       'Overview fields for travel route pages.',       true)
ON CONFLICT (slug) DO NOTHING;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, is_required, is_system,
     show_in_frontend, filterable, sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb,
       false, false, true, false, false, false, m.position, NULL
FROM custom_field_groups g
JOIN (VALUES
    ('wiki_remittance_overview',   'provider',            'Provider',             'text',   NULL, 1),
    ('wiki_remittance_overview',   'transfer_fee_usd',    'Transfer Fee (USD)',   'number', NULL, 2),
    ('wiki_remittance_overview',   'exchange_rate_margin','Exchange Rate Margin', 'text',   NULL, 3),
    ('wiki_remittance_overview',   'transfer_speed',      'Transfer Speed',       'text',   NULL, 4),
    ('wiki_remittance_overview',   'payout_methods',      'Payout Methods',       'text',   NULL, 5),
    ('wiki_remittance_overview',   'supported_corridors', 'Supported Corridors',  'text',   NULL, 6),
    ('wiki_languagetest_overview', 'test_name',           'Test Name',            'text',   NULL, 1),
    ('wiki_languagetest_overview', 'provider',            'Provider',             'text',   NULL, 2),
    ('wiki_languagetest_overview', 'format',              'Format',               'select', '["Paper","Computer","Online"]', 3),
    ('wiki_languagetest_overview', 'duration_minutes',    'Duration (minutes)',   'number', NULL, 4),
    ('wiki_languagetest_overview', 'fee_usd',             'Fee (USD)',            'number', NULL, 5),
    ('wiki_languagetest_overview', 'validity_years',      'Validity (years)',     'number', NULL, 6),
    ('wiki_travel_overview',       'route_type',          'Route Type',           'text',   NULL, 1),
    ('wiki_travel_overview',       'operator',            'Operator',             'text',   NULL, 2),
    ('wiki_travel_overview',       'duration',            'Duration',             'text',   NULL, 3),
    ('wiki_travel_overview',       'price_range',         'Price Range',          'price_range', NULL, 4),
    ('wiki_travel_overview',       'booking_link',        'Booking Link',         'url',    NULL, 5)
) AS m(grp, key, label, field_type, options, position) ON m.grp = g.slug
ON CONFLICT (group_id, key) DO NOTHING;

INSERT INTO custom_field_group_assignments (group_id, entity_class, entity_id, position)
SELECT g.id, m.cls, NULL, 0
FROM custom_field_groups g
JOIN (VALUES
    ('wiki_remittance_overview',   'remittance'),
    ('wiki_languagetest_overview', 'languagetest'),
    ('wiki_travel_overview',       'travel')
) AS m(grp, cls) ON m.grp = g.slug
WHERE NOT EXISTS (
    SELECT 1 FROM custom_field_group_assignments a
    WHERE a.group_id = g.id AND a.entity_class = m.cls AND a.entity_id IS NULL
);

INSERT INTO comparison_type_definitions (slug, name, description, field_group_id, config, is_system)
SELECT 'remittance', 'Remittance Services',
    'Compare money-transfer providers — fees, margins, speed and corridors.',
    g.id,
    '{"level":"remittance","family":"finance","item_ref_types":["remittance","comparison"],"max_item_count":5,"default_item_count":3,"allowed_nested_types":["countries"],"allow_nested_comparisons":true}'::jsonb,
    true
FROM custom_field_groups g WHERE g.slug = 'wiki_remittance_overview';

INSERT INTO comparison_type_definitions (slug, name, description, field_group_id, config, is_system)
SELECT 'language-tests', 'Language Tests',
    'Compare language proficiency tests — format, duration, fees and validity.',
    g.id,
    '{"level":"languagetest","family":"education","item_ref_types":["languagetest","comparison"],"max_item_count":5,"default_item_count":3,"allowed_nested_types":["universities","countries"],"allow_nested_comparisons":true}'::jsonb,
    true
FROM custom_field_groups g WHERE g.slug = 'wiki_languagetest_overview';

INSERT INTO comparison_type_definitions (slug, name, description, field_group_id, config, is_system)
SELECT 'travel-routes', 'Travel Routes',
    'Compare travel routes — operator, duration, price and booking.',
    g.id,
    '{"level":"travel","family":"travel","item_ref_types":["travel","comparison"],"max_item_count":5,"default_item_count":3,"allowed_nested_types":["cities","countries"],"allow_nested_comparisons":true}'::jsonb,
    true
FROM custom_field_groups g WHERE g.slug = 'wiki_travel_overview';

COMMIT;
