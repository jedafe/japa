-- 000266_seed_compare_menu_item.up.sql
-- Adds the "Compare" item to the main sidebar per Section 23 primary nav
-- seed order: ... Forum · Blog · Universities · Compare · [More overflow].
-- The comparison list endpoint is public (published+public only), so the
-- item is visible to all visitors (visibility='all').
-- Ref: japaconnect-engineering/sections/23-main-site-frontend.md
--      japaconnect-engineering/sections/30-generic-comparison-system.md

BEGIN;

INSERT INTO menu_items (tenant_id, menu_id, label, url, icon, sort_order, visibility, target, status)
SELECT '00000000-0000-0000-0000-000000000001', m.id, 'Compare', '/compare', 'git-compare', 4, 'all', '_self', 'active'
FROM menus m
WHERE m.slug = 'main_sidebar'
  AND m.tenant_id = '00000000-0000-0000-0000-000000000001'
ON CONFLICT DO NOTHING;

COMMIT;
