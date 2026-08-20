-- Rollback: remove purpose and planned_travel_date custom fields.

DELETE FROM custom_field_values
WHERE field_id IN (
    SELECT id FROM custom_fields WHERE key IN ('purpose', 'planned_travel_date')
);

DELETE FROM custom_fields
WHERE key IN ('purpose', 'planned_travel_date');
