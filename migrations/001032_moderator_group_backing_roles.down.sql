-- 001032_moderator_group_backing_roles.down.sql
-- Detaches and removes the backing roles created by 001032.

BEGIN;

DELETE FROM group_roles gr
USING user_roles r
WHERE gr.role_id = r.id
  AND r.slug IN ('blog-moderator-role', 'forum-moderators-role', 'organization-role');

DELETE FROM user_roles
WHERE slug IN ('blog-moderator-role', 'forum-moderators-role', 'organization-role');

COMMIT;
