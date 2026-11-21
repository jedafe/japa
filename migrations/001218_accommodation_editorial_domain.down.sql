-- 001218_accommodation_editorial_domain.down.sql
-- Restores the baseline listing-form shape of guide_accommodation_fields.

BEGIN;

DELETE FROM custom_fields f
USING custom_field_groups g
WHERE f.group_id = g.id
  AND g.slug = 'guide_accommodation_fields';

UPDATE custom_field_groups
SET description = 'Fields for accommodation guide content'
WHERE slug = 'guide_accommodation_fields';

INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, 'City', 'city', 'text', NULL, true, false, true, true, true, true, 1
FROM custom_field_groups g WHERE g.slug = 'guide_accommodation_fields'
ON CONFLICT (group_id, key) DO NOTHING;

INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, 'Country', 'country', 'suggest', '{"source":"geo_countries"}'::jsonb, true, false, true, true, false, true, 2
FROM custom_field_groups g WHERE g.slug = 'guide_accommodation_fields'
ON CONFLICT (group_id, key) DO NOTHING;

INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, 'Accommodation Type', 'accommodation_type', 'select',
    '["On-Campus","Off-Campus","Homestay","Shared Apartment","Studio","Hostel","Hotel"]'::jsonb,
    false, false, true, true, true, true, 3
FROM custom_field_groups g WHERE g.slug = 'guide_accommodation_fields'
ON CONFLICT (group_id, key) DO NOTHING;

INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, 'Monthly Cost (USD)', 'monthly_cost_usd', 'number', NULL, false, false, true, true, true, true, 4
FROM custom_field_groups g WHERE g.slug = 'guide_accommodation_fields'
ON CONFLICT (group_id, key) DO NOTHING;

INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, 'Deposit Required', 'deposit_required', 'boolean', NULL, false, false, true, false, false, false, 5
FROM custom_field_groups g WHERE g.slug = 'guide_accommodation_fields'
ON CONFLICT (group_id, key) DO NOTHING;

INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, 'Lease Duration', 'lease_duration', 'duration', NULL, false, false, true, false, false, false, 6
FROM custom_field_groups g WHERE g.slug = 'guide_accommodation_fields'
ON CONFLICT (group_id, key) DO NOTHING;

INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, 'Furnished', 'is_furnished', 'boolean', NULL, false, false, true, false, false, false, 7
FROM custom_field_groups g WHERE g.slug = 'guide_accommodation_fields'
ON CONFLICT (group_id, key) DO NOTHING;

INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, 'Utilities Included', 'utilities_included', 'boolean', NULL, false, false, true, false, false, false, 8
FROM custom_field_groups g WHERE g.slug = 'guide_accommodation_fields'
ON CONFLICT (group_id, key) DO NOTHING;

INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, 'Booking Link', 'booking_link', 'url', NULL, false, false, true, false, false, true, 9
FROM custom_field_groups g WHERE g.slug = 'guide_accommodation_fields'
ON CONFLICT (group_id, key) DO NOTHING;

COMMIT;
