-- 000292_grant_link_preview_rbac_node.up.sql
-- Grants the link_preview.fetch RBAC permission node (seeded in 000099/000109)
-- to the default member role so all authenticated users can use the link
-- preview endpoint. super_admin already holds the node via the 000109
-- reconciliation "grant all" step.
--
-- Ref: japaconnect-engineering/sections/15-block-editor-tiptap.md
--      japaconnect-engineering/conventions/api-routes-catalog.md (GET /link-preview)
--
-- Context: the link_preview.fetch node was seeded into the permissions table
-- by migrations 000099 and 000109, and granted to super_admin by 000109 step 2.
-- However it was never granted to the member role, and the route only enforced
-- auth.RequireAuth (not rbac.RequirePermission). This migration grants the
-- node to members so the route can be properly gated without breaking the
-- editor for regular users (BUG-008).

BEGIN;

-- Grant link_preview.fetch to the default member role (all authenticated
-- users can fetch link previews — it is a basic editor feature).
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'member'
  AND p.key = 'link_preview.fetch'
ON CONFLICT DO NOTHING;

COMMIT;
