-- 000958_seed_donate_menu_item.up.sql
-- Seeds the "Support us" nav entry for /donate (§125). The route feature-flag
-- map (000953) already maps /donate → tips.enabled; carrying feature_flag on
-- the row lets GET /menus prune it server-side when tips are off.

BEGIN;

INSERT INTO menu_items (menu_id, label, url, icon, sort_order, visibility, target, status, feature_flag)
SELECT m.id, 'Support us', '/donate', 'heart', 90, 'all', '_self', 'active', 'tips.enabled'
FROM menus m
WHERE m.slug = 'main_sidebar'
ON CONFLICT DO NOTHING;

COMMIT;
