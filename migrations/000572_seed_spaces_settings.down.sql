-- 000572_seed_spaces_settings.down.sql
-- Remove the public space settings added in 000572.

BEGIN;

DELETE FROM system_settings
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND key IN (
      'spaces.levels',
      'spaces.page_size',
      'spaces.hub_page_size',
      'spaces.forum_page_size',
      'spaces.permission_axes',
      'spaces.default_member_space_type_slug'
  );

COMMIT;
