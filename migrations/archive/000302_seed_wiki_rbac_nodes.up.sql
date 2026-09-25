-- 000302_seed_wiki_rbac_nodes.up.sql
-- Seeds the RBAC permission nodes for the Destination Wiki (§80, ADR-088).
--
-- Nodes per §80 RBAC Permissions table:
--   wiki.edit    — member with reputation ≥ threshold (reputation check in service)
--   wiki.review  — moderator, admin, super_admin
--   wiki.manage  — admin, super_admin
--   wiki.lock    — admin, super_admin
--
-- Ref: japaconnect-engineering/sections/80-destination-wiki.md
--      japaconnect-engineering/conventions/permission-key-naming.md

BEGIN;

-- 1. Seed the four wiki permission nodes.
INSERT INTO permissions (key, description)
VALUES
    ('wiki.edit',   'Submit wiki page edits (reputation-gated)'),
    ('wiki.review', 'Review and approve/reject wiki revisions (moderator+)'),
    ('wiki.manage', 'Manage wiki pages — lock, feature, generate (admin+)'),
    ('wiki.lock',   'Lock/unlock wiki pages to mod-only edits (admin+)')
ON CONFLICT (key) DO NOTHING;

-- 2. Grant wiki.edit to member+ (reputation threshold checked in service).
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('member', 'super_admin')
  AND p.key = 'wiki.edit'
ON CONFLICT DO NOTHING;

-- 3. Grant wiki.review to moderator, admin, super_admin.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('super_admin')
  AND p.key = 'wiki.review'
ON CONFLICT DO NOTHING;

-- 4. Grant wiki.manage and wiki.lock to admin, super_admin.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('super_admin')
  AND p.key IN ('wiki.manage', 'wiki.lock')
ON CONFLICT DO NOTHING;

COMMIT;
