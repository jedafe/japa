DELETE FROM custom_field_group_assignments
WHERE group_id IN (
    SELECT id FROM custom_field_groups
    WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'story_fields'
);

DELETE FROM custom_fields
WHERE group_id IN (
    SELECT id FROM custom_field_groups
    WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'story_fields'
);

DELETE FROM custom_field_groups
WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'story_fields';
