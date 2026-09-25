-- 000598_add_menu_item_feature_flag.up.sql
-- Adds optional feature_flag gating to menu items so the dashboard rail and
-- other menus can hide links for disabled platform features.
-- Ref: ADR-054, Section 42

BEGIN;

ALTER TABLE menu_items
    ADD COLUMN feature_flag text;

COMMENT ON COLUMN menu_items.feature_flag IS
    'Optional feature flag key (feature_flags.key). If set, the item is shown only when the flag is enabled on this platform.';

-- Seed feature flags for the default dashboard_rail menu so the rail respects
-- the platform feature registry (internal/features/registry.go).
UPDATE menu_items
SET feature_flag = 'feature.follow'
WHERE menu_id = (SELECT id FROM menus WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'dashboard_rail')
  AND label = 'dashboard.nav.relationships';

UPDATE menu_items
SET feature_flag = 'feature.learning'
WHERE menu_id = (SELECT id FROM menus WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'dashboard_rail')
  AND label IN ('dashboard.nav.learning', 'dashboard.nav.certificates');

UPDATE menu_items
SET feature_flag = 'feature.tickets'
WHERE menu_id = (SELECT id FROM menus WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'dashboard_rail')
  AND label = 'dashboard.nav.tickets';

UPDATE menu_items
SET feature_flag = 'feature.bookmarks'
WHERE menu_id = (SELECT id FROM menus WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'dashboard_rail')
  AND label = 'dashboard.nav.bookmarks';

UPDATE menu_items
SET feature_flag = 'feature.professional'
WHERE menu_id = (SELECT id FROM menus WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'dashboard_rail')
  AND label IN ('dashboard.nav.profiles', 'dashboard.nav.organizations');

UPDATE menu_items
SET feature_flag = 'feature.ads'
WHERE menu_id = (SELECT id FROM menus WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'dashboard_rail')
  AND label = 'dashboard.nav.analytics';

COMMIT;
