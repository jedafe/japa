-- 000238_seed_system_field_group_assignments.up.sql
--
-- The original custom field architecture migration (000008) created system
-- field groups for Blog Post, Forum, Space, University, Organization, and
-- Tag entities but never inserted the corresponding custom_field_group_assignments
-- rows. Without assignments the groups show an empty entity_type in the admin
-- panel and cannot be filtered or located by the content composer / space
-- editor / etc.
--
-- This migration inserts the missing assignment rows using the entity_class
-- values defined in Section 06 (custom-field-builder-form-builder.md):
--
--   Blog Post Fields    → content_type:blog_post
--   Forum Fields        → content_type:discussion
--   Space Fields        → space_type               (NULL entity_id = all space types)
--   University Fields   → space_type:university
--   Organization Fields → organization
--   Tag Fields          → tag
--
-- The ON CONFLICT clause makes this safe to re-run.

INSERT INTO custom_field_group_assignments (group_id, entity_class, entity_id, position)
VALUES
    ('00000000-0000-0000-0000-000000000041', 'content_type:blog_post',    NULL, 0),
    ('00000000-0000-0000-0000-000000000042', 'content_type:discussion',   NULL, 0),
    ('00000000-0000-0000-0000-000000000043', 'space_type',                NULL, 0),
    ('00000000-0000-0000-0000-000000000044', 'space_type:university',     NULL, 0),
    ('00000000-0000-0000-0000-000000000045', 'organization',              NULL, 0),
    ('00000000-0000-0000-0000-000000000046', 'tag',                       NULL, 0)
ON CONFLICT (group_id, entity_class, entity_id) DO NOTHING;
