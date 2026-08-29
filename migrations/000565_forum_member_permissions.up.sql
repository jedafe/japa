-- 000565_forum_member_permissions.up.sql
-- Fix remaining forum permission drift. Previous migrations 000144 and 000175
-- granted `forum.tag.assign`, `forum.poll.create`, `forum.tag.create`, and
-- `forum.tag.edit` to the non-existent `member_role` slug, so regular members
-- could not create polls, create tags, or assign tags to threads.
--
-- This migration also grants the `_any` and solve/tag-admin override nodes to
-- moderator and admin roles so the service-level author/moderator checks in
-- canModerateThread/canModerateReply/canSolveThread can be exercised. The
-- `POST /forum/{id}/solve` route is auth-only, but `forum.thread.solve` is still
-- the moderator/admin override inside the service.
--
-- Ref: .jeos/conventions/permission-key-naming.md Forum section
--      .jeos/sections/03-rbac.md

BEGIN;

-- 1. Self-service member+ capabilities that were missing from `member`.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('member', 'professional', 'agent', 'moderator', 'admin')
  AND p.key IN (
    'forum.poll.create',
    'forum.tag.create',
    'forum.tag.assign'
  )
ON CONFLICT DO NOTHING;

-- 2. Moderator/admin own + any nodes for thread/reply edit/delete.
-- The public routes currently require the *_own node; the service then
-- authorizes author/space-moderator or the matching *_any node. Both are
-- needed for moderator/admin to reach and pass the route/service pair.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('moderator', 'admin')
  AND p.key IN (
    'forum.thread.edit_own',
    'forum.thread.edit_any',
    'forum.thread.delete_own',
    'forum.thread.delete_any',
    'forum.reply.edit_own',
    'forum.reply.edit_any',
    'forum.reply.delete_own',
    'forum.reply.delete_any'
  )
ON CONFLICT DO NOTHING;

-- 3. Moderator/admin can also solve any thread via the service override.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('moderator', 'admin')
  AND p.key = 'forum.thread.solve'
ON CONFLICT DO NOTHING;

-- 4. Tag administration (existing routes gate on these, but no role held them).
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('admin')
  AND p.key IN (
    'forum.tag.edit',
    'forum.tag.moderate'
  )
ON CONFLICT DO NOTHING;

COMMIT;
