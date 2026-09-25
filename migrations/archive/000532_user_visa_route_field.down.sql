-- Rollback: remove the visa_route custom field for user profiles.

DELETE FROM custom_field_values
WHERE field_id IN (
    SELECT id FROM custom_fields WHERE key = 'visa_route'
);

DELETE FROM custom_fields
WHERE key = 'visa_route' AND field_type = 'select';
