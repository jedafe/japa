-- 001229_seed_digest_nav_item.up.sql
-- Adds a discoverable "Digest" entry to the seeded main_sidebar menu
-- (audit BUG-017 — /digest was only reachable via the sidebar widget).
-- The item is feature_flag-gated on digest.enabled, so the menus
-- serving layer prunes it whenever the §84 flag is off; visibility 'all'
-- matches the public /digest route.
--
-- Ref: japaconnect-engineering/sections/84-community-digest-widget.md
-- Ref: menu_items.feature_flag (gated rendering)

BEGIN;

INSERT INTO menu_items
    (id, menu_id, parent_id, label, url, icon, sort_order, permission_node,
     visibility, target, is_ad_slot, ad_slot_config, status,
     required_group_ids, feature_flag)
SELECT
    'a1e2c3d4-5f6a-4b7c-8d9e-0f1a2b3c4d5e',
    m.id, NULL, 'Digest', '/digest', 'mail', 22, NULL,
    'all', '_self', false, NULL, 'active',
    '[]'::jsonb, 'digest.enabled'
FROM menus m
WHERE m.slug = 'main_sidebar'
  AND NOT EXISTS (
      SELECT 1 FROM menu_items mi
      WHERE mi.menu_id = m.id AND mi.url = '/digest'
  );

COMMIT;
