-- Restore the geo_fields.cost_of_living field row exactly as captured
-- (same id so any external references keep working).

BEGIN;

INSERT INTO custom_fields
    (id, group_id, label, key, field_type, options, embed_platforms,
     placeholder, default_value, is_required, is_system, show_in_frontend,
     filterable, sortable, searchable, position, created_at, config, section)
SELECT
    '45390ad3-7bee-4ad1-8e5a-d92bdb9fe1ab', g.id, 'Cost of Living',
    'cost_of_living', 'text', NULL, NULL, NULL, NULL, false, true, true,
    false, false, false, 3, '2026-08-24T18:08:17.821705+00:00', '{}', NULL
FROM custom_field_groups g WHERE g.slug = 'geo_fields'
ON CONFLICT DO NOTHING;

COMMIT;
