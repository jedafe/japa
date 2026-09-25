-- 000389_seed_system_field_group_assignments.down.sql
--
-- Removes the system field group assignments added by the up migration.
-- Only deletes the rows for the specific group_id + entity_class pairs
-- inserted above; user-created assignments are untouched.

DELETE FROM custom_field_group_assignments
WHERE (group_id, entity_class) IN (
    ('00000000-0000-0000-0000-000000000041', 'content_type:blog_post'),
    ('00000000-0000-0000-0000-000000000042', 'content_type:discussion'),
    ('00000000-0000-0000-0000-000000000043', 'space_type'),
    ('00000000-0000-0000-0000-000000000044', 'space_type:university'),
    ('00000000-0000-0000-0000-000000000045', 'organization'),
    ('00000000-0000-0000-0000-000000000046', 'tag')
);
