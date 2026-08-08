-- 000291_grant_stock_rbac_nodes.up.sql
-- Grants the stock photo RBAC permission nodes (seeded in 000187) to the
-- appropriate roles and adds a dedicated media.stock.admin node for provider
-- config management.
--
-- Ref: japaconnect-engineering/sections/34-media-system.md
--      japaconnect-engineering/conventions/permission-key-naming.md
--
-- Context: migration 000187 seeded media.stock.search, media.stock.import,
-- and media.stock.providers into the permissions table but never inserted
-- any role_permissions rows. As a result no role (including member, verified
-- member, or admin roles) held the nodes — only super_admin worked via the
-- "*" wildcard. This migration grants:
--   - member: media.stock.search + media.stock.import (per 000187 comment)
--   - super_admin: all three original nodes + the new media.stock.admin node
-- The admin stock provider config routes (/admin/stock/providers*) are
-- switched from media.edit_any to media.stock.admin so that managing
-- provider API keys is not bundled with media-library edit rights.

BEGIN;

-- 1. Add the dedicated admin node for stock provider config management.
INSERT INTO permissions (key, description)
VALUES ('media.stock.admin', 'Manage stock photo provider configuration (enable/disable, API keys)')
ON CONFLICT (key) DO NOTHING;

-- 2. Grant search + import to the default member role (all authenticated users
--    can search Openverse and import stock photos into their media library).
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'member'
  AND p.key IN ('media.stock.search', 'media.stock.import')
ON CONFLICT DO NOTHING;

-- 3. Grant all four stock nodes to super_admin.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'super_admin'
  AND p.key IN (
    'media.stock.search',
    'media.stock.import',
    'media.stock.providers',
    'media.stock.admin'
  )
ON CONFLICT DO NOTHING;

COMMIT;
