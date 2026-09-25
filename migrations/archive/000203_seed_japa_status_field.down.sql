-- BUG-006: Remove the seeded Japa Status custom field.
DELETE FROM custom_fields
WHERE key = 'japa_status'
  AND group_id IN (
      SELECT id FROM custom_field_groups
      WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
        AND slug = 'user_profile_fields'
  );

DELETE FROM custom_field_group_assignments
WHERE entity_class = 'user_profile'
  AND group_id IN (
      SELECT id FROM custom_field_groups
      WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
        AND slug = 'user_profile_fields'
  );

DELETE FROM custom_field_groups
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND slug = 'user_profile_fields';
