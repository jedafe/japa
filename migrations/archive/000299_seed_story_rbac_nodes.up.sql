-- 000299_seed_story_rbac_nodes.up.sql
-- Seeds the RBAC permission nodes for the Community Story Wall (§79, ADR-087).
--
-- Nodes per §79 RBAC Permissions table:
--   story.create       — member+ (submit a story)
--   story.edit_own     — member+ (edit own story)
--   story.edit_any     — moderator, admin, super_admin
--   story.delete_own   — member+ (delete own story)
--   story.delete_any   — moderator, admin, super_admin
--   story.verify       — admin, super_admin (mark story as verified)
--
-- Ref: japaconnect-engineering/sections/79-community-story-wall.md
--      japaconnect-engineering/conventions/permission-key-naming.md

BEGIN;

-- 1. Seed the six story permission nodes.
INSERT INTO permissions (key, description)
VALUES
    ('story.create',     'Submit a community story'),
    ('story.edit_own',   'Edit own community story'),
    ('story.edit_any',   'Edit any community story (moderator+)'),
    ('story.delete_own', 'Delete own community story'),
    ('story.delete_any', 'Delete any community story (moderator+)'),
    ('story.verify',     'Mark a community story as verified (admin+)')
ON CONFLICT (key) DO NOTHING;

-- 2. Grant member-level nodes to all non-guest roles.
--    story.create, story.edit_own, story.delete_own → member, moderator, admin, super_admin
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('member', 'super_admin')
  AND p.key IN ('story.create', 'story.edit_own', 'story.delete_own')
ON CONFLICT DO NOTHING;

-- 3. Grant moderator+ nodes: story.edit_any, story.delete_any → moderator, admin, super_admin
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('super_admin')
  AND p.key IN ('story.edit_any', 'story.delete_any')
ON CONFLICT DO NOTHING;

-- 4. Grant admin+ nodes: story.verify → admin, super_admin
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('super_admin')
  AND p.key = 'story.verify'
ON CONFLICT DO NOTHING;

COMMIT;
