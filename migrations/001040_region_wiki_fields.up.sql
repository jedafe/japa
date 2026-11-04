-- 001040_region_wiki_fields.up.sql
-- §wiki-seo — region field schemas, mirroring 001035's country seeds
-- (fields + custom_field_permissions). Directory groups stay field-empty
-- like their country counterparts; overview/cost-of-living get real
-- schemas scoped to a region (no capital/currency — those are per-country).
BEGIN;

-- ── Overview (region) — region-scoped quick facts ──────────────────────────
INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('population',        'Population',         'number',   NULL, NULL, NULL, NULL, false, false, true, false, false, false, 0, 'basic_information'),
    ('countries_count',   'Countries',          'number',   NULL, NULL, NULL, NULL, false, false, true, false, false, false, 1, NULL),
    ('major_languages',   'Major Languages',    'repeater', NULL, NULL, NULL, NULL, false, false, true, false, false, false, 2, NULL),
    ('timezones',         'Time Zones',         'repeater', NULL, NULL, NULL, NULL, false, false, true, false, false, false, 3, NULL),
    ('major_cities',      'Major Cities',       'repeater', NULL, NULL, NULL, NULL, false, false, true, false, false, false, 4, NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_region_overview'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, show_in_frontend = EXCLUDED.show_in_frontend,
    position = EXCLUDED.position, section = EXCLUDED.section;

-- ── Cost of Living (region) — same monthly_expenses schema as country,
--    minus qf_currency (a region has no single currency; the rollup
--    aggregates country/state/city values underneath) ─────────────────────
INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('exp_accommodation','Accommodation','price_range','{}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,1,'monthly_expenses'),
    ('exp_food','Food','price_range','{}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,2,'monthly_expenses'),
    ('exp_transportation','Transportation','price_range','{}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,3,'monthly_expenses'),
    ('exp_utilities','Utilities','price_range','{}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,4,'monthly_expenses'),
    ('exp_internet','Internet','price_range','{}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,5,'monthly_expenses'),
    ('exp_mobile','Mobile plan','price_range','{}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,6,'monthly_expenses'),
    ('exp_entertainment','Entertainment','price_range','{}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,7,'monthly_expenses'),
    ('exp_health','Health insurance','price_range','{}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,8,'monthly_expenses'),
    ('qf_monthly_range','Average monthly cost','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,21,'quick_facts'),
    ('qf_best_for','Best for','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,22,'quick_facts')
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_region_cost_of_living'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, show_in_frontend = EXCLUDED.show_in_frontend,
    position = EXCLUDED.position, section = EXCLUDED.section;

-- ── Other (region) — the catch-all note/links pair, lighter than country ──
INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('local_tips',   'Local Tips',    'textarea', NULL, NULL, NULL, NULL, false, false, true, false, false, false, 1, NULL),
    ('useful_links', 'Useful Links',  'repeater', NULL, NULL, NULL, NULL, false, false, true, false, false, false, 2, NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_region_other'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    position = EXCLUDED.position, section = EXCLUDED.section;

-- ── Permissions — same contract as 001035: everyone views, non-guests edit ──
INSERT INTO custom_field_permissions (field_id, user_group_id, can_view, can_edit)
SELECT f.id, ug.id, true, ug.slug <> 'guest'
FROM custom_fields f
JOIN custom_field_groups g ON g.id = f.group_id
JOIN user_groups ug ON true
WHERE g.slug IN ('wiki_region_overview','wiki_region_cost_of_living','wiki_region_other')
  AND NOT EXISTS (
      SELECT 1 FROM custom_field_permissions p
      WHERE p.field_id = f.id AND p.user_group_id = ug.id
  );

COMMIT;
