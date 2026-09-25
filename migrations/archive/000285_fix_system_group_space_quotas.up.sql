-- 000285_fix_system_group_space_quotas.up.sql
-- Set max_spaces_creatable for the three system groups to match the
-- documented intent (ROADMAP-BACKEND.md line 197:
--   super_admin: -1, members: 3, guest: 0).
--
-- Background: migration 000041 added user_groups.max_spaces_creatable with
-- DEFAULT 0 but never seeded the system groups. As a result the super_admin
-- group sat at 0 ("none"), and the CreateSpace handler's quota gate
-- (internal/spaces/handlers.go §26) blocked super_admin users from creating
-- spaces with a 403 "quota_exceeded" — even though super_admin bypasses the
-- spaces.admin permission check in the RBAC middleware.
--
-- -1 = unlimited, 0 = none, N = capped at N spaces.

UPDATE user_groups
SET max_spaces_creatable = -1
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND slug = 'super_admin';

UPDATE user_groups
SET max_spaces_creatable = 3
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND slug = 'members';

UPDATE user_groups
SET max_spaces_creatable = 0
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND slug = 'guest';
