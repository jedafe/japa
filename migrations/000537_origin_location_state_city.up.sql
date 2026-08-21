-- 000537_origin_location_state_city.up.sql
-- ADR-064a: extend the origin location model with state and city precision.
-- The settings/location page now uses cascading Country → State → City pickers
-- for the user's origin, so we need custom fields to persist the selected
-- state (a spaces.id UUID) and city (a spaces.id UUID) alongside the existing
-- origin_country ISO code.

INSERT INTO custom_fields (id, group_id, tenant_id, label, key, field_type, is_system, is_required, position)
VALUES
    ('00000000-0000-0000-0000-000000000058',
     '00000000-0000-0000-0000-000000000040',
     '00000000-0000-0000-0000-000000000001',
     'Origin State', 'origin_state_id', 'text',
     true, false, 3),
    ('00000000-0000-0000-0000-000000000059',
     '00000000-0000-0000-0000-000000000040',
     '00000000-0000-0000-0000-000000000001',
     'Origin City', 'origin_city_id', 'text',
     true, false, 4)
ON CONFLICT (id) DO NOTHING;
