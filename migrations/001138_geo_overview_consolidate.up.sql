-- Consolidate the geo picker's "Geo Fields" + "Overview" rail groups.
-- The bound geo_fields profile group and the pooled wiki_*_overview
-- group overlapped (population, capital, timezone, currency…). Copy the
-- geo-only fields into each overview group so nothing is lost, drop the
-- geo_fields pool rows so only "Overview" renders, and remove the
-- truncated-key duplicate official_languag inside wiki_country_overview
-- (0 values, 0 dimensions; official_language is canonical).
--
-- Fields NOT copied (already covered semantically in the target):
--   cities:    population, time_zone  (population, timezone exist)
--   countries: population, capital, language, currency, calling_code,
--              time_zone (dup/official_language/timezones exist)
--   states:    capital, population, language, time_zone
-- geo_fields itself stays bound as the entity profile group — only its
-- pool membership is removed (3 rows at position -1).

BEGIN;

-- wiki_city_overview: append 9 geo-only fields
INSERT INTO custom_fields (group_id, label, key, field_type, options, embed_platforms, placeholder, default_value, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position, config, section)
SELECT ov.id, cf.label, cf.key, cf.field_type, cf.options, cf.embed_platforms, cf.placeholder, cf.default_value, cf.is_required, cf.is_system, cf.show_in_frontend, cf.filterable, cf.sortable, cf.searchable,
       base.max_pos + ROW_NUMBER() OVER (ORDER BY cf.position) AS position,
       cf.config, cf.section
FROM custom_fields cf
JOIN custom_field_groups geo ON geo.id = cf.group_id AND geo.slug = 'geo_fields'
JOIN custom_field_groups ov ON ov.slug = 'wiki_city_overview'
CROSS JOIN LATERAL (SELECT MAX(position) AS max_pos FROM custom_fields f WHERE f.group_id = ov.id) base
WHERE cf.key IN ('area', 'capital', 'language', 'currency', 'postal_codes', 'calling_code', 'official_website', 'founded', 'head_of_government');

-- wiki_country_overview: append 5 geo-only fields
INSERT INTO custom_fields (group_id, label, key, field_type, options, embed_platforms, placeholder, default_value, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position, config, section)
SELECT ov.id, cf.label, cf.key, cf.field_type, cf.options, cf.embed_platforms, cf.placeholder, cf.default_value, cf.is_required, cf.is_system, cf.show_in_frontend, cf.filterable, cf.sortable, cf.searchable,
       base.max_pos + ROW_NUMBER() OVER (ORDER BY cf.position) AS position,
       cf.config, cf.section
FROM custom_fields cf
JOIN custom_field_groups geo ON geo.id = cf.group_id AND geo.slug = 'geo_fields'
JOIN custom_field_groups ov ON ov.slug = 'wiki_country_overview'
CROSS JOIN LATERAL (SELECT MAX(position) AS max_pos FROM custom_fields f WHERE f.group_id = ov.id) base
WHERE cf.key IN ('area', 'postal_codes', 'official_website', 'founded', 'head_of_government');

-- wiki_state_overview: append 7 geo-only fields
INSERT INTO custom_fields (group_id, label, key, field_type, options, embed_platforms, placeholder, default_value, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position, config, section)
SELECT ov.id, cf.label, cf.key, cf.field_type, cf.options, cf.embed_platforms, cf.placeholder, cf.default_value, cf.is_required, cf.is_system, cf.show_in_frontend, cf.filterable, cf.sortable, cf.searchable,
       base.max_pos + ROW_NUMBER() OVER (ORDER BY cf.position) AS position,
       cf.config, cf.section
FROM custom_fields cf
JOIN custom_field_groups geo ON geo.id = cf.group_id AND geo.slug = 'geo_fields'
JOIN custom_field_groups ov ON ov.slug = 'wiki_state_overview'
CROSS JOIN LATERAL (SELECT MAX(position) AS max_pos FROM custom_fields f WHERE f.group_id = ov.id) base
WHERE cf.key IN ('area', 'currency', 'postal_codes', 'calling_code', 'official_website', 'founded', 'head_of_government');

-- Remove geo_fields from the three pickers' pools (bound group untouched)
DELETE FROM comparison_type_field_groups
WHERE group_id = '4dfd79c2-d087-4e5c-80b1-bb10b5f90b89'
  AND type_id IN (SELECT id FROM comparison_type_definitions WHERE slug IN ('cities', 'countries', 'states'));

-- Truncated-key duplicate inside wiki_country_overview
DELETE FROM custom_fields WHERE id = '04b8b098-2822-4682-bf32-326ab00d309b';

COMMIT;
