-- 000537_origin_location_state_city.down.sql
-- Rollback: remove origin_state_id and origin_city_id custom fields and any
-- values that were stored against them.

DELETE FROM custom_field_values WHERE field_id IN (
    '00000000-0000-0000-0000-000000000058',
    '00000000-0000-0000-0000-000000000059'
);

DELETE FROM custom_fields WHERE id IN (
    '00000000-0000-0000-0000-000000000058',
    '00000000-0000-0000-0000-000000000059'
);
