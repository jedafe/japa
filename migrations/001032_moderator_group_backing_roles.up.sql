-- 001032_moderator_group_backing_roles.up.sql
-- The seeded groups blog-moderator, forum-moderators and organization have no
-- attached roles — joining them grants nothing, and they show as empty shells
-- in the RBAC admin UI. Create a backing role for each ("<Group> Role",
-- matching the groupPermissionRole convention) and grant the JEOS-documented
-- default nodes:
--
--   forum-moderators  §08/§49 moderator defaults
--   blog-moderator    blog.review.manage + §49 comments moderation defaults
--   organization      §98/§111 org_member defaults (claim/create/booking-manage
--                     on the owning org — service.booking.manage is org-scoped
--                     in the service layer)
--
-- Grants are inserted by permission key; rows are idempotent.

BEGIN;

-- 1) Backing roles (one per group).
INSERT INTO user_roles (id, name, slug, created_at, updated_at) VALUES
    (gen_random_uuid(), 'Blog Moderator Role',  'blog-moderator-role',  now(), now()),
    (gen_random_uuid(), 'Forum Moderators Role','forum-moderators-role',now(), now()),
    (gen_random_uuid(), 'Organization Role',    'organization-role',    now(), now())
ON CONFLICT (slug) DO NOTHING;

-- 2) Attach roles to their groups.
INSERT INTO group_roles (group_id, role_id)
SELECT g.id, r.id
FROM user_groups g
JOIN user_roles r ON r.slug = 'blog-moderator-role'
WHERE g.slug = 'blog-moderator'
ON CONFLICT DO NOTHING;

INSERT INTO group_roles (group_id, role_id)
SELECT g.id, r.id
FROM user_groups g
JOIN user_roles r ON r.slug = 'forum-moderators-role'
WHERE g.slug = 'forum-moderators'
ON CONFLICT DO NOTHING;

INSERT INTO group_roles (group_id, role_id)
SELECT g.id, r.id
FROM user_groups g
JOIN user_roles r ON r.slug = 'organization-role'
WHERE g.slug = 'organization'
ON CONFLICT DO NOTHING;

-- 3) Grants.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM user_roles r, permissions p
WHERE r.slug = 'forum-moderators-role'
  AND p.key IN (
    'forum.thread.edit_any',
    'forum.thread.delete_any',
    'forum.thread.solve',
    'forum.reply.edit_any',
    'forum.reply.delete_any',
    'forum.tag.moderate',
    'forum.ama.moderate',
    'comments.edit_any',
    'comments.delete_any',
    'comments.pin',
    'moderation.content.review'
  )
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM user_roles r, permissions p
WHERE r.slug = 'blog-moderator-role'
  AND p.key IN (
    'blog.review.manage',
    'comments.edit_any',
    'comments.delete_any',
    'comments.pin'
  )
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM user_roles r, permissions p
WHERE r.slug = 'organization-role'
  AND p.key IN (
    'service.claim',
    'service.create',
    'service.booking.manage',
    'community_locator.claim'
  )
ON CONFLICT DO NOTHING;

COMMIT;
